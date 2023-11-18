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
	"time"
)

type GeneralAgentInfo struct {
	SystemName     string
	Settings       *AgentSettings
	Server         *os_client.AgentOSClient
	InputVariables map[string]any
	History        []*engines.Message // no need to keep track of turn numbers - only replyTo is important
}

func (agent *GeneralAgentInfo) ParseResponse(response string) ([]*ResponseParserResult, error) {
	return agent.Settings.ParseResponse(response)
}

func NewGeneralAgentState(client *os_client.AgentOSClient, systemName string, config *AgentSettings) *GeneralAgentInfo {
	if systemName == "" {
		systemName = tools.GetSystemName(config.Agent.Name)
	}
	return &GeneralAgentInfo{
		SystemName:     systemName,
		Settings:       config,
		Server:         client,
		InputVariables: map[string]any{},
		History:        make([]*engines.Message, 0),
	}
}

// GeneralAgentPipelineStep engine inference schema:
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
func GeneralAgentPipelineStep(agentState *GeneralAgentInfo,
	currentDepth, // current depth of history, 0 - means only system prompt
	batchSize, // try to create this many jobs
	maxSamplingAttempts, // how many times we can try to sample `batchSize` jobs
	minResults int, // how many inference results before using only cached inference
) ([]*engines.Message, error) {
	// let's get context right
	if agentState.Settings == nil || agentState.Settings.Agent == nil || agentState.Settings.Agent.PromptBased == nil ||
		agentState.Settings.Agent.PromptBased.Prompt == "" {
		return nil, fmt.Errorf("not a pormpt-based agent - empty prompt in agent settings")
	}

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
	messageId := GenerateMessageId(contextString)
	systemMessage := &engines.Message{
		Role:    engines.ChatRoleSystem,
		Content: contextString,
		ID:      &messageId,
	}

	// ok, now, we always start with system message
	chats := make([][]*engines.Message, 0, batchSize)
	jobs := make([]cmds.GetCompletionRequest, 0, batchSize)
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
		jobs = append(jobs, cmds.GetCompletionRequest{
			RawPrompt:   chatToRawPrompt(chat),
			MinResults:  minResults,
			Temperature: 0.9,
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
	serverResponse, err := agentState.Server.RunRequest(&cmds.ClientRequest{
		ProcessName:           agentState.SystemName,
		Priority:              borrow_engine.PRIO_User,
		GetCompletionRequests: jobs,
	}, 600*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error getting completion: %v", err)
	}

	// whatever it is, it's an assistant's messages, so we should add those
	// to our history, with respect to replyTo field
	resultMessages := make([]*engines.Message, 0, len(serverResponse.GetCompletionResponse))
	for jobResultIdx, jobResult := range serverResponse.GetCompletionResponse {
		// what was the last message in jobs[jobResultIdx]
		lastMessageId := *chats[jobResultIdx][len(chats[jobResultIdx])-1].ID

		for _, choice := range jobResult.Choices {
			thisMessageId := GenerateMessageId(choice)
			resultMessage := &engines.Message{
				ID:      &thisMessageId,
				Content: choice,
				ReplyTo: &lastMessageId,
				Role:    engines.ChatRoleAssistant,
			}
			agentState.History = append(agentState.History, resultMessage)
			resultMessages = append(resultMessages, resultMessage)
			// at this point it's clear what to parse and where to put the response / observation, etc
		}
	}

	return resultMessages, nil
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
