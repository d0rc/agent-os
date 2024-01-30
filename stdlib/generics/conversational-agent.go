package generics

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	agent_tools "github.com/d0rc/agent-os/stdlib/agent-tools"
	os_client "github.com/d0rc/agent-os/stdlib/os-client"
	"strings"
	"sync"
)

type SimpleAgent struct {
	client             *os_client.AgentOSClient
	description        string
	goal               string
	name               string
	history            []*engines.Message
	lock               sync.RWMutex
	tools              []agent_tools.AgentTool
	responseFields     []ResponseField
	processingFunction func(resp map[string]interface{}, choice string) (interface{}, error)
}

func NewConversationalAgent(client *os_client.AgentOSClient, description, name, goal string) *SimpleAgent {
	return &SimpleAgent{
		client:             client,
		description:        description,
		name:               name,
		goal:               goal,
		history:            []*engines.Message{},
		lock:               sync.RWMutex{},
		tools:              []agent_tools.AgentTool{},
		responseFields:     []ResponseField{},
		processingFunction: nil,
	}
}

func (ca *SimpleAgent) WithStringResponseField(name, content string) *SimpleAgent {
	ca.lock.Lock()
	defer ca.lock.Unlock()

	ca.responseFields = append(ca.responseFields, StringResponseField(name, content))
	return ca
}

func (ca *SimpleAgent) WithIntResponseField(name, content string, min, max int) *SimpleAgent {
	ca.lock.Lock()
	defer ca.lock.Unlock()

	ca.responseFields = append(ca.responseFields, IntResponseField(name, content, min, max))
	return ca
}

func (ca *SimpleAgent) WithStructuredResponseField(name string, structure []ResponseField) *SimpleAgent {
	ca.lock.Lock()
	defer ca.lock.Unlock()

	ca.responseFields = append(ca.responseFields, StructuredResponseField(name, structure))
	return ca
}

func (ca *SimpleAgent) WithResponseProcessor(f func(resp map[string]interface{}, choice string) (interface{}, error)) *SimpleAgent {
	ca.lock.Lock()
	defer ca.lock.Unlock()

	ca.processingFunction = f
	return ca
}

func (ca *SimpleAgent) ReceiveMessage(msg *engines.Message) {
	ca.lock.Lock()
	ca.history = append(ca.history, msg)
	ca.lock.Unlock()
}

func (ca *SimpleAgent) GetName() string {
	return ca.name
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
	err := CreateSimplePipeline(ca.client, ca.name).
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
