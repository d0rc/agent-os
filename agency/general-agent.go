package agency

import (
	"crypto/sha512"
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	pongo2 "github.com/flosch/pongo2/v6"
	"github.com/google/uuid"
	"math/rand"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

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
	quitHistoryAppeneder   chan struct{}
	historySize            int32
}

func (agentState *GeneralAgentInfo) ParseResponse(response string) ([]*ResponseParserResult, error) {
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
		jobsChannel:              make(chan *cmds.ClientRequest, 100),
		resultsChannel:           make(chan *cmds.ServerResponse, 100),
		resultsProcessingChannel: make(chan *engines.Message, 100),
		//ioProcessingChannel:      make(chan *cmds.ClientRequest, 100),
		historyAppenderChannel: make(chan *engines.Message, 100),
		quitChannelJobs:        make(chan struct{}, 1),
		quitChannelResults:     make(chan struct{}, 1),
		quitChannelProcessing:  make(chan struct{}, 1),
		//quitIoProcessing:         make(chan struct{}, 1),
		quitHistoryAppeneder: make(chan struct{}, 1),
	}

	go agentState.agentStateJobsSender()
	go agentState.agentStateResultsReceiver()
	go agentState.resultsProcessing()
	//go agentState.ioProcessing()
	go agentState.historyAppender()

	return agentState
}

func (agentState *GeneralAgentInfo) historyAppender() {
	for {
		select {
		case <-agentState.quitHistoryAppeneder:
			return
		case message := <-agentState.historyAppenderChannel:
			agentState.History = append(agentState.History, message)
			atomic.AddInt32(&agentState.historySize, 1)
		}
	}
}

func (agentState *GeneralAgentInfo) agentStateJobsSender() {
	for {
		select {
		case <-agentState.quitChannelJobs:
			return
		case job := <-agentState.jobsChannel:
			go func(job *cmds.ClientRequest) {
				resp, err := agentState.Server.RunRequest(job, 600*time.Second)
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
			if serverResult.GetCompletionResponse != nil &&
				len(serverResult.GetCompletionResponse) > 0 {
				for _, jobResult := range serverResult.GetCompletionResponse {
					for _, choice := range jobResult.Choices {
						thisMessageId := GenerateMessageId(choice)
						resultMessage := &engines.Message{
							ID:      &thisMessageId,
							Content: choice,
							ReplyTo: &serverResult.CorrelationId,
							Role:    engines.ChatRoleAssistant,
						}
						agentState.historyAppenderChannel <- resultMessage
						agentState.resultsProcessingChannel <- resultMessage
					}
				}
			}
		}
	}
}

func (agentState *GeneralAgentInfo) resultsProcessing() {
	for {
		select {
		case <-agentState.quitChannelProcessing:
			return
		case message := <-agentState.resultsProcessingChannel:
			go func(message *engines.Message) {
				ioRequests := agentState.TranslateToServerCalls([]*engines.Message{message})
				// run all at once
				ioResponses, err := agentState.Server.RunRequests(ioRequests, 600*time.Second)
				if err != nil {
					fmt.Printf("error running IO request: %v\n", err)
					return
				}

				// fmt.Printf("Got responses: %v\n", res)
				// we've got responses, if we have observations let's put them into the history
				for idx, commandResponse := range ioResponses {
					for _, observation := range generateObservationFromServerResults(ioRequests[idx], commandResponse, 1024) {
						messageId := GenerateMessageId(observation)
						agentState.historyAppenderChannel <- &engines.Message{
							ID:      &messageId,
							ReplyTo: &commandResponse.CorrelationId, // it should be equal to message.ID TODO: check
							Role:    engines.ChatRoleUser,
							Content: observation,
						}
					}
				}
			}(message)
		}
	}
}

func generateObservationFromServerResults(request *cmds.ClientRequest, response *cmds.ServerResponse, maxLength int) []string {
	observations := make([]string, 0)
	observation := ""
	if request.SpecialCaseResponse != "" {
		observations = append(observations, request.SpecialCaseResponse)
		return observations
	}

	if len(response.GoogleSearchResponse) > 0 {
		for _, searchResponse := range response.GoogleSearchResponse {
			//observation += fmt.Sprintf("Search results for \"%s\":\n", searchResponse.Keywords)
			for _, searchResult := range searchResponse.URLSearchInfos {
				observation += fmt.Sprintf("%s\n%s\n%s\n\n", searchResult.Title, searchResult.URL, searchResult.Snippet)
				if len(observation) > maxLength {
					observations = append(observations, observation)
					observation = ""
				}
			}
		}
	}

	if observation != "" {
		observations = append(observations, observation)
	}

	return observations
}

func (agentState *GeneralAgentInfo) Stop() {
	agentState.quitChannelJobs <- struct{}{}
	agentState.quitChannelResults <- struct{}{}
	agentState.quitChannelProcessing <- struct{}{}
	agentState.quitHistoryAppeneder <- struct{}{}
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
		tpl, err := pongo2.FromString(agentState.Settings.Agent.PromptBased.Prompt)
		if err != nil {
			return fmt.Errorf("error parsing agent's prompt: %v", err)
		}

		contextString, err := tpl.Execute(agentState.InputVariables)
		if err != nil {
			return fmt.Errorf("error executing agent's prompt: %v", err)
		}
		// result is a System message...!
		responseFormat := agentState.Settings.GetResponseJSONFormat()

		contextString = fmt.Sprintf("%s\nRespond always in JSON format:\n%s\n", contextString, responseFormat)
		messageId := GenerateMessageId(contextString)
		systemMessage := &engines.Message{
			Role:    engines.ChatRoleSystem,
			Content: contextString,
			ID:      &messageId,
		}

		// ok, now, we always start with system message
		chats := make([][]*engines.Message, 0, batchSize)
		jobs := make([]cmds.ClientRequest, 0, batchSize)
		samplingAttempt := 0
		for {
			samplingAttempt++
			if samplingAttempt > maxSamplingAttempts {
				break
			}
			if len(jobs) >= batchSize {
				break
			}
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
				messageToAdd := options[randomInt(len(options))]
				chat = append(chat, messageToAdd)
			}

			if chat == nil || len(chat) == 0 {
				continue
			}
			if chat[len(chat)-1].Role != engines.ChatRoleAssistant {
				chats = append(chats, chat)
			}
		}

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

		fmt.Printf("Running inference for %d chats, min len: %d, max len: %d\n", len(chats), minLen, maxLen)
		// now we have jobs, let's run them
		originalHistorySize := atomic.LoadInt32(&agentState.historySize)
		for _, job := range jobs {
			agentState.jobsChannel <- &job
		}

		// no we have to wait until we've got at least len(jobs) new history messages
		for {
			historySize := atomic.LoadInt32(&agentState.historySize)
			if historySize >= originalHistorySize+int32(len(jobs)) {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func GenerateMessageId(body string) string {
	return uuid.NewHash(sha512.New(), uuid.Nil, []byte(body), 5).String()
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

func randomInt(max int) int {
	// generate value
	return rand.Int() % max
}
