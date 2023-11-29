package agency

import (
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	pongo2 "github.com/flosch/pongo2/v6"
	"math/rand"
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
				if *storedMessage.ID == *messageId {
					fmt.Printf("got message with ID %s already in history, merging replyTo maps\n", *messageId)
					if storedMessage.Content != message.Content {
						storedMessage.Content = message.Content // fatal error!
					}
					alreadyExists = true
					for k, _ := range message.ReplyTo {
						storedMessage.ReplyTo[k] = message.ReplyTo[k]
					}
				}
			}
			if !alreadyExists {
				fmt.Printf("Adding new message to history: %s\n", *messageId)
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
							ReplyTo: map[string]struct{}{serverResult.CorrelationId: {}},
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

func (agentState *GeneralAgentInfo) visitTerminalMessage(messages []*engines.Message) bool {
	// first let's check how many times we've been here
	chainSignature := getChatSignature(messages)

	agentState.terminalsLock.Lock()
	defer agentState.terminalsLock.Unlock()

	timesVisited, exists := agentState.terminalsVisitsMap[chainSignature]
	if exists && timesVisited > 15 {
		// we've been here more than 5 times, let's remove it
		return false
	}

	votesRating, exists := agentState.terminalsVotesMap[chainSignature]
	if exists && votesRating < 5.0 {
		// it's not worth visiting at all
		return false
	}

	// in any other case - start voting...!
	if messages[len(messages)-1].Role == engines.ChatRoleAssistant {
		/*
			votes, err := agentState.VoteForAction(messages[0].Content, messages[len(messages)-1].Content)
			if err != nil {
				return
			}

			agentState.terminalsVotesMap[chainSignature] = votes
			agentState.terminalsVisitsMap[chainSignature] = timesVisited + 1
		*/
		return false
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
				MinResults:  9,
				Temperature: 0.9,
			},
		},
		CorrelationId: *messages[len(messages)-1].ID,
	}

	return true
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
		signature += *msg.ID + ":"
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
