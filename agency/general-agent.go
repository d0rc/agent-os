package agency

import (
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	pongo2 "github.com/flosch/pongo2/v6"
	"github.com/logrusorgru/aurora"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ResultsNumberDelta if nothing interesting got generated, increase number of _new_ results
const ResultsNumberDelta = 10

type GeneralAgentInfo struct {
	SystemName               string
	Settings                 *AgentSettings
	Server                   *os_client.AgentOSClient
	InputVariables           map[string]any
	History                  []*engines.Message // no need to keep track of turn numbers - only replyTo is important
	jobsChannel              chan *cmds.ClientRequest
	resultsChannel           chan *cmds.ServerResponse
	quitChannelJobs          chan struct{}
	quitChannelResults       chan struct{}
	resultsProcessingChannel chan *engines.Message
	quitChannelProcessing    chan struct{}
	//quitIoProcessing         chan struct{}
	//ioProcessingChannel      chan *cmds.ClientRequest
	historyAppenderChannel chan *engines.Message
	quitHistoryAppender    chan struct{}
	historySize            int32

	terminalsLock      sync.RWMutex
	terminalsVisitsMap map[string]int
	terminalsVotesMap  map[string]float32
}

func (agentState *GeneralAgentInfo) ParseResponse(response string) ([]*ResponseParserResult, string, error) {
	return agentState.Settings.ParseResponse(response)
}

func NewGeneralAgentState(client *os_client.AgentOSClient, systemName string, config *AgentSettings) *GeneralAgentInfo {
	if systemName == "" {
		systemName = tools.GetSystemName(config.Agent.Name)
	}
	agentState := &GeneralAgentInfo{
		SystemName:               systemName,
		Settings:                 config,
		Server:                   client,
		InputVariables:           map[string]any{},
		History:                  make([]*engines.Message, 0),
		jobsChannel:              make(chan *cmds.ClientRequest, 1),
		resultsChannel:           make(chan *cmds.ServerResponse, 100),
		resultsProcessingChannel: make(chan *engines.Message, 100),
		//ioProcessingChannel:      make(chan *cmds.ClientRequest, 100),
		historyAppenderChannel: make(chan *engines.Message, 100),
		quitChannelJobs:        make(chan struct{}, 1),
		quitChannelResults:     make(chan struct{}, 1),
		quitChannelProcessing:  make(chan struct{}, 1),
		//quitIoProcessing:         make(chan struct{}, 1),
		quitHistoryAppender: make(chan struct{}, 1),

		terminalsVisitsMap: make(map[string]int),
		terminalsVotesMap:  make(map[string]float32),
		terminalsLock:      sync.RWMutex{},
	}

	go agentState.agentStateJobsSender()
	go agentState.agentStateResultsReceiver()
	go agentState.ioRequestsProcessing()
	//go agentState.ioProcessing()
	go agentState.historyAppender()

	return agentState
}

func (agentState *GeneralAgentInfo) historyAppender() {
	for {
		select {
		case <-agentState.quitHistoryAppender:
			return
		case message := <-agentState.historyAppenderChannel:
			// let's see if message already in the History
			messageId := message.ID
			alreadyExists := false
			for _, storedMessage := range agentState.History {
				if storedMessage.ID == messageId {
					alreadyExists = true
					break
				}
			}
			if !alreadyExists {
				agentState.History = append(agentState.History, message)
				atomic.AddInt32(&agentState.historySize, 1)
			}
		}
	}
}

func (agentState *GeneralAgentInfo) agentStateJobsSender() {
	maxJobThreads := make(chan struct{}, 4)
	for {
		select {
		case <-agentState.quitChannelJobs:
			return
		case job := <-agentState.jobsChannel:
			go func(job *cmds.ClientRequest) {
				maxJobThreads <- struct{}{}
				defer func() {
					<-maxJobThreads
				}()
				resp, err := agentState.Server.RunRequest(job, 600*time.Second, os_client.REP_Default)
				if err != nil {
					fmt.Printf("error running request: %v\n", err)
				}
				agentState.resultsChannel <- resp
			}(job)
		}
	}
}

func (agentState *GeneralAgentInfo) agentStateResultsReceiver() {
	for {
		select {
		case <-agentState.quitChannelResults:
			return
		case serverResult := <-agentState.resultsChannel:
			if serverResult != nil && serverResult.GetCompletionResponse != nil &&
				len(serverResult.GetCompletionResponse) > 0 {
				for _, jobResult := range serverResult.GetCompletionResponse {
					for _, choice := range jobResult.Choices {
						thisMessageId := engines.GenerateMessageId(choice)
						resultMessage := &engines.Message{
							ID:      &thisMessageId,
							Content: choice,
							ReplyTo: &serverResult.CorrelationId,
							Role:    engines.ChatRoleAssistant,
						}
						agentState.resultsProcessingChannel <- resultMessage
					}
				}
			}
		}
	}
}

func (agentState *GeneralAgentInfo) Stop() {
	agentState.quitChannelJobs <- struct{}{}
	agentState.quitChannelResults <- struct{}{}
	agentState.quitChannelProcessing <- struct{}{}
	agentState.quitHistoryAppender <- struct{}{}
}

// GeneralAgentPipelineRun engine inference schema:
// make inference:
//   - create context
//   - append known messages from current history
//   - get inference result
//
// if it's not an api call, use observation pipeline
// if it's an observation call - use observation pipeline
// if it's a context modification api call:
//   - fork agent and history context
//   - use observation pipeline
//
// an observation pipeline:
//   - append observation and toggle next inference
//   - retry current generation if limit of success runs is not reached
func (agentState *GeneralAgentInfo) GeneralAgentPipelineRun(
	batchSize, // try to create this many jobs
	maxSamplingAttempts, // how many times we can try to sample `batchSize` jobs
	minResults int, // how many inference results before using only cached inference
) error {
	// let's get context right
	if agentState.Settings == nil || agentState.Settings.Agent == nil || agentState.Settings.Agent.PromptBased == nil ||
		agentState.Settings.Agent.PromptBased.Prompt == "" {
		return fmt.Errorf("not a pormpt-based agent - empty prompt in agent settings")
	}

	for {
		systemMessage, err := agentState.getSystemMessage()
		if err != nil {
			return err
		}

		searchTs := time.Now()
		// ok, now, we always start with system message
		chats := make([][]*engines.Message, 0, batchSize)
		chatsChannel := make(chan []*engines.Message, 16384)
		jobs := make([]cmds.ClientRequest, 0, batchSize)
		samplingAttempt := 0
		maxParallelThreads := make(chan struct{}, runtime.NumCPU()-1)
		doneChannel := make(chan struct{}, 1)
		wg := sync.WaitGroup{}
		go func() {
			for {
				samplingAttempt++
				maxParallelThreads <- struct{}{}
				wg.Add(1)
				chatsFound := int32(0)
				go func() {
					defer func() {
						<-maxParallelThreads
						wg.Done()
					}()
					chat := make([]*engines.Message, 0)
					chat = append(chat, systemMessage)
					for {
						options := make([]*engines.Message, 0)
						for _, msg := range agentState.History {
							if *msg.ReplyTo == *chat[len(chat)-1].ID {
								options = append(options, msg)
							}
						}
						// ok, now we have len(options) messages to choose from
						if len(options) == 0 {
							// it's the end of the thread, continue to the next one
							break
						}
						// pick a random message from options
						// let's see if these messages are agent's action choices...
						options = agentState.applyCrossRoadsMagic(options)
						messageToAdd := options[randomInt(len(options))]
						chat = append(chat, messageToAdd)
					}

					if chat == nil || len(chat) == 0 {
						return
					}
					if chat[len(chat)-1].Role != engines.ChatRoleAssistant {
						// chats = append(chats, chat)
						chatsChannel <- chat
						atomic.AddInt32(&chatsFound, 1)
					}
				}()

				if samplingAttempt > maxSamplingAttempts {
					doneChannel <- struct{}{}
					if atomic.LoadInt32(&chatsFound) == 0 {
						// we've failed to find any chat to continue with
						// let's try from system message again
						chatsChannel <- []*engines.Message{
							systemMessage,
						}
						minResults += ResultsNumberDelta
					}
					break
				}
				if len(chats) >= batchSize {
					doneChannel <- struct{}{}
					break
				}
			}
		}()

		for {
			done := false
			select {
			case chat := <-chatsChannel:
				chats = append(chats, chat)
			case <-doneChannel:
				done = true
			}

			if done {
				break
			}
		}

		wg.Wait()

		if chats == nil || len(chats) == 0 {
			continue
		}
		// no same chats in the same batch
		chats = deDupeChats(chats)

		// now pick the top maxBatchSize chats
		// so sort chats by the lebgth
		// and pick the top batchSize
		sort.Slice(chats, func(i, j int) bool {
			return len(chats[i]) < len(chats[j])
		})
		if len(chats) > batchSize {
			chats = chats[:batchSize]
		}

		for _, chat := range chats {
			jobs = append(jobs, cmds.ClientRequest{
				ProcessName: agentState.SystemName,
				Priority:    borrow_engine.PRIO_User,
				GetCompletionRequests: []cmds.GetCompletionRequest{
					{
						RawPrompt:   chatToRawPrompt(chat),
						MinResults:  minResults,
						Temperature: 0.9,
					},
				},
				CorrelationId: *chat[len(chat)-1].ID,
			})
		}

		// start inference
		// min length of chats, max length of chats
		minLen := len(chats[0])
		maxLen := len(chats[0])
		for _, chat := range chats {
			if len(chat) < minLen {
				minLen = len(chat)
			}
			if len(chat) > maxLen {
				maxLen = len(chat)
			}
		}

		fmt.Printf("Running inference for %d chats, min len: %d, max len: %d, search took: %v\n", len(chats), minLen, maxLen,
			time.Since(searchTs))
		// now we have jobs, let's run them
		originalHistorySize := atomic.LoadInt32(&agentState.historySize)
		for _, job := range jobs {
			agentState.jobsChannel <- &job
		}

		// no we have to wait until we've got at least len(jobs) new history messages
		for {
			historySize := atomic.LoadInt32(&agentState.historySize)
			if historySize >= originalHistorySize+int32(len(jobs)/2) {
				fmt.Printf("Got %d/%d new messages in history, going to continue searching language space\n",
					aurora.BrightBlue(historySize-originalHistorySize),
					aurora.BrightCyan(historySize))
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func (agentState *GeneralAgentInfo) getSystemMessage() (*engines.Message, error) {
	tpl, err := pongo2.FromString(agentState.Settings.Agent.PromptBased.Prompt)
	if err != nil {
		return nil, fmt.Errorf("error parsing agent's prompt: %v", err)
	}

	contextString, err := tpl.Execute(agentState.InputVariables)
	if err != nil {
		return nil, fmt.Errorf("error executing agent's prompt: %v", err)
	}
	// result is a System message...!
	responseFormat := agentState.Settings.GetResponseJSONFormat()

	contextString = fmt.Sprintf("%s\nRespond always in JSON format:\n%s\n", contextString, responseFormat)
	messageId := engines.GenerateMessageId(contextString)
	systemMessage := &engines.Message{
		Role:    engines.ChatRoleSystem,
		Content: contextString,
		ID:      &messageId,
	}
	return systemMessage, nil
}

func (agentState *GeneralAgentInfo) visitTerminalMessage(messages []*engines.Message) {
	// first let's check how many times we've been here
	chainSignature := getChatSignature(messages)

	agentState.terminalsLock.Lock()
	defer agentState.terminalsLock.Unlock()

	timesVisited, exists := agentState.terminalsVisitsMap[chainSignature]
	if exists && timesVisited > 5 {
		// we've been here more than 5 times, let's remove it
		return
	}

	votesRating, exists := agentState.terminalsVotesMap[chainSignature]
	if exists && votesRating < 5.0 {
		// it's not worth visiting at all
		return
	}

	// in any other case - start voting...!
	if messages[len(messages)-1].Role == engines.ChatRoleAssistant {
		votes, err := agentState.VoteForAction(messages[0].Content, messages[len(messages)-1].Content)
		if err != nil {
			return
		}

		agentState.terminalsVotesMap[chainSignature] = votes
		agentState.terminalsVisitsMap[chainSignature] = timesVisited + 1
	} else {
		agentState.terminalsVotesMap[chainSignature] = 10 // it's real-world input, don't ignore just yet...!
		agentState.terminalsVisitsMap[chainSignature] = timesVisited + 1
	}

	// if we've got here, we can go on...!
	agentState.jobsChannel <- &cmds.ClientRequest{
		ProcessName: agentState.SystemName,
		Priority:    borrow_engine.PRIO_User,
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:   chatToRawPrompt(messages),
				MinResults:  8,
				Temperature: 0.9,
			},
		},
		CorrelationId: *messages[len(messages)-1].ID,
	}
}

func deDupeChats(chats [][]*engines.Message) [][]*engines.Message {
	chatSignatures := make(map[string]struct{})
	deDupedChats := make([][]*engines.Message, 0)
	for _, chat := range chats {
		if len(chat) == 1 {
			deDupedChats = append(deDupedChats, chat)
			continue
		}
		chatSignature := getChatSignature(chat)
		if _, exists := chatSignatures[chatSignature]; !exists {
			chatSignatures[chatSignature] = struct{}{}
			deDupedChats = append(deDupedChats, chat)
		}
	}

	return deDupedChats
}

func getChatSignature(chat []*engines.Message) string {
	signature := ""
	for _, msg := range chat {
		signature += *msg.ID
	}

	return signature
}

func chatToRawPrompt(sample []*engines.Message) string {
	// following well known ### Instruction ### Assistant ### User format
	rawPrompt := strings.Builder{}
	for _, message := range sample {
		switch message.Role {
		case engines.ChatRoleSystem:
			rawPrompt.WriteString(fmt.Sprintf("### Instruction:\n%s\n", message.Content))
		case engines.ChatRoleAssistant:
			rawPrompt.WriteString(fmt.Sprintf("### Assistant:\n%s\n", message.Content))
		case engines.ChatRoleUser:
			rawPrompt.WriteString(fmt.Sprintf("### User:\n%s\n", message.Content))
		}
	}
	rawPrompt.WriteString("### Assistant:\n")

	return rawPrompt.String()
}

var source = rand.NewSource(1)
var rng = rand.New(source)
var rngLock = sync.RWMutex{}

func randomInt(max int) int {
	// generate value
	rngLock.Lock()
	defer rngLock.Unlock()
	return rng.Int() % max
}
