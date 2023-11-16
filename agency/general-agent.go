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
	"strings"
	"time"
)

type GeneralAgentInfo struct {
	SystemName string
	Settings   *AgentSettings
	Server     *os_client.AgentOSClient
}

func (agent *GeneralAgentInfo) ParseResponse(response string) ([]*ResponseParserResult, error) {
	return agent.Settings.ParseResponse(response)
}

type InferenceContext struct {
	InputVariables map[string]any
	History        [][]*engines.Message
}

func NewGeneralAgentState(client *os_client.AgentOSClient, systemName string, config *AgentSettings) *GeneralAgentInfo {
	if systemName == "" {
		systemName = tools.GetSystemName(config.Agent.Name)
	}
	return &GeneralAgentInfo{
		SystemName: systemName,
		Settings:   config,
		Server:     client,
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
func GeneralAgentPipelineStep(state *GeneralAgentInfo,
	currentDepth, // current depth of history, 0 - means only system prompt
	batchSize, // try to create this many jobs
	maxSamplingAttempts, // how many times we can try to sample `batchSize` jobs
	minResults int, // how many inference results before using only cached inference
	history *InferenceContext) ([]*engines.Message, error) {
	// let's get context right
	if state.Settings == nil || state.Settings.Agent == nil || state.Settings.Agent.PromptBased == nil ||
		state.Settings.Agent.PromptBased.Prompt == "" {
		return nil, fmt.Errorf("not a pormpt-based agent - empty prompt in agent settings")
	}

	tpl, err := pongo2.FromString(state.Settings.Agent.PromptBased.Prompt)
	if err != nil {
		return nil, fmt.Errorf("error parsing agent's prompt: %v", err)
	}

	contextString, err := tpl.Execute(history.InputVariables)
	if err != nil {
		return nil, fmt.Errorf("error executing agent's prompt: %v", err)
	}
	// result is a System message...!
	responseFormat, err := state.Settings.GetResponseJSONFormat()
	if err != nil {
		return nil, fmt.Errorf("error getting response format: %v", err)
	}

	contextString = fmt.Sprintf("%s\nRespond always in JSON format:\n%s\n", contextString, responseFormat)
	messageId := uuid.NewHash(sha512.New(), uuid.Nil, []byte(contextString), 5).String()
	systemMessage := &engines.Message{
		Role:    engines.ChatRoleSystem,
		Content: contextString,
		ID:      &messageId,
	}

	// now let's create a required batch of chat requests on our current level of history
	jobs := make([]cmds.GetCompletionRequest, 0, batchSize)
	jobsSelectedMessageId := make([]string, 0, batchSize)
	jobsSelectedMessageLevel := make([]int, 0, batchSize)
	// sample batchSize histories from current history, up to the current depth
	// the thing is each message, except for #0 is something which was replied a level deeper
	// so, we can choose a random thread of messages, picking on current level those, which are
	// marked as a reply to message on previous level, and if we're on the level 0, we can pick
	// any message -- that's it
	// let's start sampling!
	samplingAttempts := 0
	for {
		if currentDepth == 0 || len(history.History) == 0 {
			// it's just a system message, nothing to sample, just break
			// whatever they want - it's impossible
			break
		}

		samplingAttempts++
		if samplingAttempts > maxSamplingAttempts {
			// we can't do it anymore
			break
		}
		if len(jobs) >= batchSize {
			// we're done
			break
		}
		currentSample := make([]*engines.Message, 0, currentDepth)

		// let's pick a random message from history
		randomMessage := history.History[0][randomInt(len(history.History))]
		currentSample = append(currentSample, randomMessage)

		attemptFailed := false
		if currentDepth > 0 {
			for i := 1; i < currentDepth; i++ {
				// what was the message on previous level
				previousMessageId := currentSample[i-1].ID
				// let's find all messages on current level, which are replies to previous message
				options := make([]*engines.Message, 0, len(history.History[i]))
				for _, message := range history.History[i] {
					if message.ReplyTo == previousMessageId {
						options = append(options, message)
					}
				}
				if len(options) == 0 {
					// we can't do it anymore
					attemptFailed = true
					break
				}

				// pick a random message from options
				currentSample = append(currentSample, options[randomInt(len(options))])
			}
		}

		if attemptFailed {
			continue
		}

		// we've got a sample, let's make a request
		jobs = append(jobs, cmds.GetCompletionRequest{
			RawPrompt:   chatToRawPrompt(currentSample),
			MinResults:  minResults,
			Temperature: 0.8,
		})
		if currentSample[len(currentSample)-1].ID == nil {
			// make an id for this message
			prevMessageId := uuid.NewHash(sha512.New(), uuid.Nil, []byte(currentSample[len(currentSample)-1].Content), 5).String()
			currentSample[len(currentSample)-1].ID = &prevMessageId
		}
		jobsSelectedMessageId = append(jobsSelectedMessageId, *currentSample[len(currentSample)-1].ID)
		jobsSelectedMessageLevel = append(jobsSelectedMessageLevel, len(currentSample)-1)
	}

	if len(jobs) == 0 {
		// it's just a system message, make batchSize of them
		for i := 0; i < batchSize; i++ {
			jobs = append(jobs, cmds.GetCompletionRequest{
				RawPrompt:   chatToRawPrompt([]*engines.Message{systemMessage}),
				MinResults:  minResults,
				Temperature: 0.8,
			})
			jobsSelectedMessageId = append(jobsSelectedMessageId, *systemMessage.ID)
			jobsSelectedMessageLevel = append(jobsSelectedMessageLevel, 0)
		}
	}

	// start inference
	serverResponse, err := state.Server.RunRequest(&cmds.ClientRequest{
		ProcessName:           state.SystemName,
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
		lastMessageId := jobsSelectedMessageId[jobResultIdx]
		lastMessageLevel := jobsSelectedMessageLevel[jobResultIdx]
		if len(history.History) == 0 {
			history.History = append(history.History, make([]*engines.Message, 0))
		}
		if history.History[lastMessageLevel] == nil {
			history.History[lastMessageLevel] = make([]*engines.Message, 0, len(jobResult.Choices))
		}
		for _, choice := range jobResult.Choices {
			thisMessageId := uuid.NewHash(sha512.New(), uuid.Nil, []byte(choice), 5).String()
			resultMessage := &engines.Message{
				ID:      &thisMessageId,
				Content: choice,
				ReplyTo: &lastMessageId,
				Role:    engines.ChatRoleAssistant,
			}
			history.History[lastMessageLevel] = append(history.History[lastMessageLevel], resultMessage)
			resultMessages = append(resultMessages, resultMessage)
			// at this point it's clear what to parse and where to put the response / observation, etc
		}
	}

	return resultMessages, nil
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
