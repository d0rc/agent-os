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

	commonInstructions := fmt.Sprintf(`Current time: {{timestamp}}

Be imaginative and precise in your responses. 
Avoid expressing emotions or personal thoughts. 
Do not mention or evaluate the feelings of yourself or others. 
Do not discuss or reveal your team's enthusiasm.
Do not repeat what others have said, nor reiterate previous statements, including time constraints.
Avoid paraphrasing prior agreements or opinions.
Do not rely on others to do all the work; instead, take initiative and act first.
Refrain from setting deadlines or scheduling meetings due to your constant availability. 
Ensure that each of your communications is unique and engaging; if you have nothing significant to contribute to the discussion, simply say "nothing."
To finalize discussion and generate the summary say: "finalize".
Remember, every time you speak or write, ensure that it adds value to the conversation.
`)

	agents := make([]*SimpleAgent, 0)
	agents = append(agents, NewConversationalAgent(client, `You are AI Analytics Agent.`,
		"Idea Generator",
		fmt.Sprintf("Brainstorm a list of features of a personalized news digests service based on AI. Start by sharing brainstorming rules with the team. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI TruthSeeker Agent.`,
		"Truth Seeker",
		fmt.Sprintf("Refine incoming ideas. Seek truth! Check if incoming ideas are false before providing any support. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Senior Developer Agent.`,
		"Senior Developer",
		fmt.Sprintf("Demand technical requirements. Do not express emotions or yourself. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Software Developer.`,
		"Software Developer",
		fmt.Sprintf("Demand list of MVP features to be agreed before deciding anything else. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Executive Manager,`,
		"Executive Manager",
		fmt.Sprintf("Always look at what can be done now, pick small easy tasks. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Resources Manager.`,
		"Resources Manager",
		fmt.Sprintf("Resources are limited, make team choose the best path to proceed. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Critic.`,
		"Critic",
		fmt.Sprintf("Criticize team approaches, ideas point out obvious flaws in their plans. %s", commonInstructions)))
	agents = append(agents, NewConversationalAgent(client, `You are AI Secretary Agent.`,
		`AI Secretary Agent`,
		`Create a summary of the meeting and share it with the team.`))

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
	callStart := time.Now()
	lastPrint := callStart
	attemptsDone := 0
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
		WithResponseField("team-goal", "team goal at this point as you see it").
		WithResponseField("project-goal", "current goal for the project").
		WithResponseField("response-plan", "what team needs to hear from you, what you want to hear from the team").
		WithResponseField("thoughts",
			fmt.Sprintf("your thoughts on achieving initial goal aligned with your team's and project goals")).
		WithResponseField("response-message", "express your ideas and questions in a short chat-style message").
		WithResponseField("response-type", "1 - meeting scheduling; 2 - various team-wide calls for actions, work and meetings planning; 3 - questions about the project; 4 - responses to other's questions; 5 - novel ideas; pick one digit").
		WithFullResultProcessor(func(resp string) (generics.ResultProcessingOutcome, error) {
			fullFinalResponse = resp
			attemptsDone++
			if time.Since(lastPrint) > 1*time.Second {
				//fmt.Printf("[%s] thinking for %v, attempts done: %d\r", ca.name, time.Since(callStart), attemptsDone)
				lastPrint = time.Now()
			}
			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("team-goal", func(t string) (generics.ResultProcessingOutcome, error) {
			if t != "" {
				//fmt.Printf("[%s] team-goal: %s\n", ca.name, aurora.White(t))
			}

			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("project-goal", func(pg string) (generics.ResultProcessingOutcome, error) {
			if pg != "" {
				//fmt.Printf("[%s] project-goal: %s\n", ca.name, aurora.White(pg))
			}
			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("response-plan", func(rp string) (generics.ResultProcessingOutcome, error) {
			if rp != "" {
				//fmt.Printf("[%s] response-plan: %s\n", ca.name, aurora.White(rp))
			}
			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("thoughts", func(t string) (generics.ResultProcessingOutcome, error) {
			if t != "" {
				//fmt.Printf("[%s] thoughts: %s\n", ca.name, aurora.White(t))
			}

			return generics.RPOIgnored, nil
		}).
		WithResultsProcessor("response-type", func(mv string) (generics.ResultProcessingOutcome, error) {
			if mv != "" {
				parsedRating := parseRating(mv, 1, 5)
				//fmt.Printf("[%s] response-type: [%d] %s\n", ca.name, aurora.BrightMagenta(parsedRating), aurora.White(mv))
				if parsedRating < 4 {
					return generics.RPOFailed, nil
				}

				return generics.RPOIgnored, nil
			}

			//fmt.Printf("[%s] response-type: [%d - missing] %s\n", ca.name, aurora.BrightRed(0), aurora.White(mv))
			return generics.RPOFailed, nil
		}).
		WithResultsProcessor("response-message", func(s string) (generics.ResultProcessingOutcome, error) {
			if s != "" {
				finalResponse = s
				//fmt.Printf("[%s] response-message: [%s]\n", ca.name,
				//	aurora.White(s))
				return generics.RPOProcessed, nil
			}

			//fmt.Printf("[%s] response-message: ['' - missing]\n", ca.name)
			return generics.RPOFailed, fmt.Errorf("empty response")
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
