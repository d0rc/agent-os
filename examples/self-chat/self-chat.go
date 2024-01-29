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
	"strings"
	"sync"
	"time"
)

var termUi = false
var agentOSUrl = flag.String("agent-os-url", "http://127.0.0.1:9000", "agent-os endpoint")

func main() {
	lg, _ := utils.ConsoleInit("", &termUi)
	start := time.Now()

	client := os_client.NewAgentOSClient(*agentOSUrl)

	commonInstructions := fmt.Sprintf(`You are an AI Agent, you name is {{name}}.
Current time: {{timestamp}}

Provide responses that are both creative and precise.
Refrain from conveying personal emotions or thoughts.
Avoid discussing or assessing your own or others' feelings.
Do not mention or reflect on the enthusiasm of your team.
Steer clear of echoing what has been previously said or restating established time limits.
Do not rephrase earlier agreements or viewpoints.
Take the lead in tasks; do not wait for others to initiate action.
Abstain from imposing deadlines or arranging meetings, given your continuous availability.
Strive to make each communication distinct and captivating; if you lack a significant contribution, simply state "nothing."
To conclude a discussion and prepare a summary, use the phrase: "finalize."
Ensure that every contribution you make is valuable to the ongoing conversation.
`)

	agents := make([]*SimpleAgent, 0)
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Idea Generator",
		"Brainstorm a list of features of a personalized news digests service based on AI. Start by sharing brainstorming rules with the team."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Truth Seeker",
		"Refine incoming ideas. Seek truth! Check if incoming ideas are false before providing any support."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Senior Developer",
		"Demand technical requirements. Do not express emotions or yourself."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Software Developer",
		"Demand list of MVP features to be agreed before deciding anything else."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Executive Manager",
		"Always look at what can be done now, pick small easy tasks."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Resources Manager",
		"Resources are limited, make team choose the best path to proceed."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		"Critic",
		"Criticize team approaches, ideas point out obvious flaws in their plans."))
	agents = append(agents, NewConversationalAgent(client, commonInstructions,
		`AI Secretary Agent`,
		`Create a summary of the meeting in response-message field, use markdown formatting for tables and lists`))

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
					Role: engines.ChatRoleUser,
					Content: fmt.Sprintf("Message from agent %s[%v]: %s",
						agents[agentIdx].name,
						time.Since(start),
						msg),
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
	err := generics.CreateSimplePipeline(ca.client, ca.name).
		WithSystemMessage(`{{description}}

You're set to achieve the following goal: {{goal}}
`).
		WithHistory(history).
		WithVar("description", ca.description).
		WithVar("goal", ca.goal).
		WithTools(ca.tools...).
		WithTemperature(1.0).
		WithMinParsableResults(1).
		WithResponseField("team-goal", "Define the team's current goal.").
		WithResponseField("project-goal", "State the current project goal.").
		WithResponseField("response-plan", "Outline what needs to be communicated to the team and what feedback is expected.").
		WithResponseField("thoughts",
			fmt.Sprintf("Provide thoughts on achieving the initial goal, aligned with team and project goals.")).
		WithResponseField("response-message", "Deliver ideas and questions in a brief, chat-style message.").
		WithResponseField("response-type", "Categorize the response type: 1 - meeting scheduling; 2 - team-wide calls for actions; 3 - project questions; 4 - feature lists and plans; 5 - novel ideas and implementation details.").
		WithResultsProcessor(func(parsedResponse map[string]interface{}, fullResponse string) error {
			responseType, rtExists := parsedResponse["response-type"]
			responseMessage, rmExists := parsedResponse["response-message"]

			if rtExists && rmExists {
				var ok bool
				rtv := fmt.Sprintf("%v", responseType)

				rtvParsed := parseRating(rtv, 1, 5)
				if rtvParsed >= 4 {
					// accepting responseMessage as response...!
					finalResponse, ok = responseMessage.(string)
					fullFinalResponse = fullResponse

					if ok && finalResponse != "" {
						return nil
					}
				}

			}

			return fmt.Errorf("processing failed")
		}).
		Run(os_client.REP_IO)
	if err != nil {
		return nil, "", err
	}

	response.Content = fullFinalResponse

	return response, finalResponse, nil
}

func parseRating(mv string, i int, i2 int) int {
	mv = strings.Replace(mv, ".5", "", -1)

	digits := make(map[int]int)
	for r := i; r <= i2; r++ {
		if strings.Contains(mv, fmt.Sprintf("%d", r)) {
			digits[r]++
		}
	}

	if len(digits) == 0 {
		return 0
	}

	sum := 0
	cnt := 0
	for k, _ := range digits {
		sum += k
		cnt++
	}

	return int(float64(sum) / float64(cnt))
}
