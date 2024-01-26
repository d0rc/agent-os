package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/engines"
	agent_tools "github.com/d0rc/agent-os/stdlib/agent-tools"
	"github.com/d0rc/agent-os/stdlib/generics"
	os_client "github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/syslib/utils"
	"github.com/logrusorgru/aurora"
	"sync"
)

var termUi = false
var agentOSUrl = flag.String("agent-os-url", "http://127.0.0.1:9000", "agent-os endpoint")

func main() {
	lg, _ := utils.ConsoleInit("", &termUi)

	client := os_client.NewAgentOSClient(*agentOSUrl)

	commonInstructions := `Be creative and specific. 
Do not express emotions or yourself. 
Do not reflect your or others emotions and feelings. 
Don't reflect or share your or team's excitement.
If you have nothing to add, just say "agreed"".`

	agents := make([]*SimpleAgent, 0)
	agents = append(agents, NewConversationalAgent(client, `You are AI Analytics Agent.`,
		"Idea Generator",
		fmt.Sprintf("Initiate and support a discussion on a feature list of an operating system for AI Agents' enviroments. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI TruthSeeker Agent.`,
		"Truth Seeker",
		fmt.Sprintf("Refine incoming ideas. Seek truth! Check if incoming ideas are false before providing any support. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI CTO Agent.`,
		"CTO",
		fmt.Sprintf("Demand technical requirements. Do not express emotions or yourself. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Software Developer.`,
		"Software Developer",
		fmt.Sprintf("Demand list of MVP features to be agreed before deciding anything else. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `Executive Manager`,
		"Executive Manager",
		fmt.Sprintf("Always look at what can be done now, pick small easy tasks. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `"Resources Manager`,
		"Resources Manager",
		fmt.Sprintf("Resources are limited, make team choose the best path to proceed. %s", commonInstructions)))

	for {
		for agentIdx, _ := range agents {
			res, msg, err := agents[agentIdx].ProcessInput("")
			if err != nil {
				lg.Fatal().Err(err).Msgf("error processing input")
			}

			agents[agentIdx].ReceiveMessage(res)
			fmt.Printf("Agent [%s]: %s\n", aurora.BrightBlue(agents[agentIdx].name), aurora.BrightYellow(msg))
			for subAgentIdx, _ := range agents {
				if subAgentIdx == agentIdx {
					continue
				}

				agents[subAgentIdx].ReceiveMessage(&engines.Message{
					Role:    engines.ChatRoleUser,
					Content: fmt.Sprintf("Message from agent %d: %s", agents[agentIdx].name, msg),
				})
			}
		}
	}
}

type SimpleAgent struct {
	client      *os_client.AgentOSClient
	description string
	goal        string
	name        string
	history     []*engines.Message
	lock        sync.RWMutex
	tools       []agent_tools.AgentTool
}

func NewConversationalAgent(client *os_client.AgentOSClient, description, name, goal string) *SimpleAgent {
	return &SimpleAgent{
		client:      client,
		description: description,
		name:        name,
		goal:        goal,
		history:     []*engines.Message{},
		lock:        sync.RWMutex{},
		tools:       []agent_tools.AgentTool{},
	}
}

func (ca *SimpleAgent) ReceiveMessage(msg *engines.Message) {
	ca.lock.Lock()
	ca.history = append(ca.history, msg)
	ca.lock.Unlock()
}

func (ca *SimpleAgent) GetHistory() []*engines.Message {
	history := make([]*engines.Message, len(ca.history))
	ca.lock.RLock()

	for i, msg := range ca.history {
		history[i] = msg
	}

	ca.lock.RUnlock()

	return history
}

func (ca *SimpleAgent) ProcessInput(input string) (*engines.Message, string, error) {
	history := ca.GetHistory()
	if input != "" {
		history = append(history, &engines.Message{
			Role:    engines.ChatRoleUser,
			Content: input,
		})
	}

	response := &engines.Message{
		Role: engines.ChatRoleAssistant,
	}

	finalResponse := ""
	fullFinalResponse := ""
	err := generics.CreateSimplePipeline(ca.client).
		WithSystemMessage(`{{description}}

You're set to achieve the following goal: {{goal}}
`).
		WithHistory(history).
		WithVar("description", ca.description).
		WithVar("goal", ca.goal).
		WithTools(ca.tools...).
		WithTemperature(1.0).
		WithMinParsableResults(1).
		WithResponseField("thoughts",
			fmt.Sprintf("your thoughts on how to steer communication in order to achieve your initial goal: %s", ca.goal)).
		WithResponseField("response-message", "a message to send").
		WithFullResultProcessor(func(resp string) (generics.ResultProcessingOutcome, error) {
			fullFinalResponse = resp
			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("thoughts", func(t string) (generics.ResultProcessingOutcome, error) {
			if t != "" {
				fmt.Printf("thoughts: %s\n", aurora.White(t))
			}

			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("response-message", func(s string) (generics.ResultProcessingOutcome, error) {
			if s != "" {
				finalResponse = s
				return generics.RPOProcessed, nil
			}

			return generics.RPOFailed, fmt.Errorf("empty response")
		}).
		Run(os_client.REP_IO)
	if err != nil {
		return nil, "", err
	}

	response.Content = fullFinalResponse

	return response, finalResponse, nil
}
