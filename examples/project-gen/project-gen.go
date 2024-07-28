package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/stdlib/generics"
	os_client "github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/syslib/utils"
	"github.com/logrusorgru/aurora"
	"sort"
)

var termUi = false
var agentOSUrl = flag.String("agent-os-url", "http://127.0.0.1:9000", "agent-os endpoint")

func main() {
	lg, _ := utils.ConsoleInit("", &termUi)

	client := os_client.NewAgentOSClient(*agentOSUrl)

	creationInstructions := make(chan string, 1024)
	go func() {
		_ = generics.CreateSimplePipeline(client, "job-postings-generator").
			WithSystemMessage("You are God-like AGI's executive AI Agent. Do whatever requested by User.").
			WithUserMessage("Please elaborate what it takes to create a virtual hosting platform to resell other cloud providers, like an Uber but for hosting. Focus on technical side of things. Market Research is not our problem. Remember - we're not creating virtualization platform, we're just creating re-seller platform, and we're not only going to integrate all existing cloud providers we can think of, but we should provide an open API in order to allow smaller providers to use our platform to reach their customers.").
			WithMinParsableResults(50).
			WithResultsProcessor(func(parsed map[string]interface{}, full string) error {
				//lg.Info().Msgf("results: `%s`", full)
				creationInstructions <- full
				return nil
			}).
			Run(os_client.REP_IO)

	}()

	roles := make(chan interface{}, 1024)

	go func() {
		for instruction := range creationInstructions {
			_ = generics.CreateSimplePipeline(client, "contractors-list-creator").
				WithSystemMessage(`You are God-like AGI's executive AI Agent. Your goal is to create a list of required roles and contractors. 

Use JSON format:
[
	{
		"role": "role name",
		"description": "role description"
    }
]
`).
				WithUserMessage(instruction).
				WithMinParsableResults(50).
				WithSliceOfResultsProcessor(func(parsed []interface{}, full string) error {
					if parsed != nil && len(parsed) > 0 {
						for _, role := range parsed {
							roles <- role
						}
					}

					return nil
				}).
				Run(os_client.REP_IO)
		}
	}()

	rolesTop := make(map[string]int)
	for role := range roles {
		roleMap := role.(map[string]interface{})
		roleName := roleMap["role"].(string)
		if _, ok := rolesTop[roleName]; ok {
			rolesTop[roleName]++
		} else {
			rolesTop[roleName] = 1
			fmt.Printf("%v: %v\n",
				aurora.BrightCyan(roleMap["role"]),
				aurora.White(roleMap["description"]))
		}

		tableSlice := make([]struct {
			s string
			n int
		}, 0)
		for k, v := range rolesTop {
			tableSlice = append(tableSlice, struct {
				s string
				n int
			}{k, v})
		}

		// sort tableSlice by n, lowest - first
		sort.Slice(tableSlice, func(i, j int) bool {
			return tableSlice[i].n < tableSlice[j].n
		})

		fmt.Printf("%v\n",
			aurora.BrightCyan("--------------------------------------------------"))
		for i := 0; i < len(tableSlice); i++ {
			fmt.Printf("%v: %v\n",
				aurora.BrightCyan(tableSlice[i].s),
				aurora.White(tableSlice[i].n))
		}

	}

	lg.Info().Msgf("Done...")
}
