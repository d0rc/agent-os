package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/stdlib/generics"
	os_client "github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/syslib/utils"
	"github.com/logrusorgru/aurora"
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

	/*ideaGenerator := generics.NewConversationalAgent(client, commonInstructions,
	"Idea Generator",
	"Brainstorm a list of features of personalized news digest service based on AI. Start by sharing brainstorming rules with the team.").
	WithStringResponseField("thoughts", "your thoughts on the topic").
	WithIntResponseField("response-type", "", 1, 5).
	WithStructuredResponseField("command", generics.ResponseStructure(
		generics.StringResponseField("name", "command name"),
		generics.StructuredResponseField("args", nil))).
	WithStringResponseField("thoughts", "your thoughts on the topic").
	WithStringResponseField("response-message", "your message to your team").
	WithResponseProcessor(func(resp map[string]interface{}, choice string) (interface{}, error) {
		return map[string]interface{}{
			"thoughts":         resp["thoughts"],
			"response-message": resp["response-message"],
		}, nil
	})*/

	agents := make([]*generics.SimpleAgent, 0)
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Idea Generator",
		"Brainstorm a list of features of a personalized news digests service based on AI. Start by sharing brainstorming rules with the team."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Truth Seeker",
		"Refine incoming ideas. Seek truth! Check if incoming ideas are false before providing any support."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Senior Developer",
		"Demand technical requirements. Do not express emotions or yourself."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Software Developer",
		"Demand list of MVP features to be agreed before deciding anything else."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Executive Manager",
		"Always look at what can be done now, pick small easy tasks."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Resources Manager",
		"Resources are limited, make team choose the best path to proceed."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		"Critic",
		"Criticize team approaches, ideas point out obvious flaws in their plans."))
	agents = append(agents, generics.NewConversationalAgent(client, commonInstructions,
		`AI Secretary Agent`,
		`Create a summary of the meeting in response-message field, use markdown formatting for tables and lists`))

	for {
		for agentIdx, _ := range agents {
			res, msg, err := agents[agentIdx].ProcessInput("")
			if err != nil {
				lg.Fatal().Err(err).Msgf("error processing input")
			}

			agents[agentIdx].ReceiveMessage(res)
			fmt.Printf("Agent [%s]: %s\n", aurora.BrightBlue(agents[agentIdx].GetName()), aurora.BrightYellow(msg))
			for subAgentIdx, _ := range agents {
				if subAgentIdx == agentIdx {
					continue
				}

				agents[subAgentIdx].ReceiveMessage(&engines.Message{
					Role: engines.ChatRoleUser,
					Content: fmt.Sprintf("Message from agent %s[%v]: %s",
						agents[agentIdx].GetName(),
						time.Since(start),
						msg),
				})
			}
		}
	}
}
