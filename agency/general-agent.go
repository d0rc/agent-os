package agency

import (
	"fmt"
	message_store "github.com/d0rc/agent-os/message-store"
	"sync"
	"time"

	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	pongo2 "github.com/flosch/pongo2/v6"
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
	systemWriterChannel    chan *engines.Message
	quitHistoryAppender    chan struct{}
	historySize            int32

	terminalsLock      sync.RWMutex
	terminalsVisitsMap map[string]int
	terminalsVotesMap  map[string]float32

	ForkCallback       func(name, goal string) chan string
	FinalReportChannel chan string
	jobsSubmittedTs    time.Time
	jobsReceived       uint64
	jobsFinished       uint64
	historyUpdated     chan struct{}

	waitLock          sync.Mutex
	waitingResponseTo map[string]int

	space *message_store.SemanticSpace
}

func (agentState *GeneralAgentInfo) ParseResponse(response string) ([]*ResponseParserResult, string, string, error) {
	return agentState.Settings.ParseResponse(response)
}

const (
	IV_GOAL = "goal"
)

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
		resultsChannel:           make(chan *cmds.ServerResponse, 1024),
		resultsProcessingChannel: make(chan *engines.Message, 100),
		//ioProcessingChannel:      make(chan *cmds.ClientRequest, 100),
		historyAppenderChannel: make(chan *engines.Message, 100),
		quitChannelJobs:        make(chan struct{}, 1),
		quitChannelResults:     make(chan struct{}, 1),
		quitChannelProcessing:  make(chan struct{}, 1),
		historyUpdated:         make(chan struct{}, 1),
		//quitIoProcessing:         make(chan struct{}, 1),
		quitHistoryAppender: make(chan struct{}, 1),
		systemWriterChannel: make(chan *engines.Message, 100),

		terminalsVisitsMap: make(map[string]int),
		terminalsVotesMap:  make(map[string]float32),
		terminalsLock:      sync.RWMutex{},

		ForkCallback:       nil,
		FinalReportChannel: nil,
		jobsSubmittedTs:    time.Now(),
		jobsReceived:       0,
		jobsFinished:       0,

		waitLock:          sync.Mutex{},
		waitingResponseTo: make(map[string]int),

		space: message_store.NewSemanticSpace(3),
	}

	// copy all input variables from the config
	for k, v := range config.Agent.PromptBased.Vars {
		agentState.InputVariables[k] = v
	}

	go agentState.jobsChannelManager()
	go agentState.agentStateResultsReceiver()
	go agentState.ioRequestsProcessing()
	//go agentState.ioProcessing()
	go agentState.historyAppender()

	return agentState
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
	agentState.systemWriterChannel <- systemMessage
	return systemMessage, nil
}

func getChatSignature(chat []*engines.Message) string {
	signature := ""
	for _, msg := range chat {
		signature += *msg.ID + ":"
	}

	return signature
}
