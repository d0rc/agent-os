package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"github.com/d0rc/agent-os/utils"
	"os"
	"time"
)

//go:embed agency.yaml
var agencyYaml []byte

var termUi = false

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting research-agency-1")

	agencySettings, err := agency.ParseAgency(agencyYaml)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to parse agency")
	}

	lg.Info().Interface("agencySettings", agencySettings).Msg("parsed agency")

	//client := os_client.NewAgentOSClient("http://167.235.115.231:9000")
	client := os_client.NewAgentOSClient("http://127.0.0.1:9000")
	agentState := agency.NewGeneralAgentState(client, "", agencySettings[0])

	var spawningCallback func(name, goal string) chan string
	spawningCallback = func(name, goal string) chan string {
		clonedSettings, err := agency.ParseAgency(agencyYaml)
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to parse agency")
		}

		clonedSettings[0].Agent.Name = name
		clonedSettings[0].Agent.PromptBased.Vars[agency.IV_GOAL] = goal
		newAgentState := agency.NewGeneralAgentState(client, "", clonedSettings[0])
		finalReportsStream := make(chan string, 10)

		newAgentState.FinalReportChannel = finalReportsStream
		newAgentState.ForkCallback = spawningCallback
		go newAgentState.ToTPipeline()

		return finalReportsStream
	}

	agentState.ForkCallback = spawningCallback
	finalReportsStream := make(chan string, 1024)
	agentState.FinalReportChannel = finalReportsStream
	go agentState.ToTPipeline()

	finalReports := make([]string, 0)
	for finalReport := range finalReportsStream {
		fmt.Println(finalReport)
		finalReports = append(finalReports, finalReport)
		if len(finalReports) > 2 {
			ratings := make(map[int]int)
			for idxA, reportA := range finalReports {
				for idxB, reportB := range finalReports {
					if idxA == idxB {
						continue
					}
					if reportA == reportB {
						continue
					}

					yes, err := isReportABetter(client, agentState.InputVariables["goal"].(string), reportA, reportB)
					if err != nil {
						continue
					}
					if yes {
						ratings[idxA]++
						//ratings[idxB]--
					}

					no, err := isReportABetter(client, agentState.InputVariables["goal"].(string), reportB, reportA)
					if err != nil {
						continue
					}
					if no {
						ratings[idxB]++
						//ratings[idxA]--
					}
				}
			}

			fmt.Printf("ratings: %v\n", ratings)

			// print reports from worth to best
			printReports(ratings, finalReports)
		}
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}

func printReports(ratings map[int]int, reports []string) {
	minVal := ratings[0]
	minIdx := 0
	maxVal := ratings[0]
	maxIdx := 0
	for idx, val := range ratings {
		if val < minVal {
			minVal = val
			minIdx = idx
		}
		if val > maxVal {
			maxVal = val
			maxIdx = idx
		}
	}
	// create a string writer
	sw := fmt.Sprintf(`
Best report, has score of %d:
%s

Least scored report, has score of %d:
%s
`, maxVal, codeblock(reports[maxIdx]), minVal, codeblock(reports[minIdx]))
	_ = os.WriteFile("reports-table.txt", []byte(fmt.Sprintf("Total reports: %d\n", len(reports))+sw), 0644)
}

func isReportABetter(client *os_client.AgentOSClient, goal string, a string, b string) (bool, error) {
	prompt := `### Instruction:
Primary goal:
%s

Your task is to compare following two reports:
Report A:
%s

Report B:
%s

Which of the reports is more comprehensive and better aligns with the primary goal?
Provide response in the following JSON format:

%s
{
    "thoughts": "thoughts text, discussing which report is more comprehensive and better aligns with the primary goal",
    "best-report": "<A|B>"
}
%s
### Assistant: `
	type modelResponse struct {
		BestReport string `json:"best-report"`
	}
	parsedResponse := modelResponse{}
	prompt = fmt.Sprintf(prompt, codeblock(goal), codeblock(a), codeblock(b), "```", "```")
	minResults := 5
retry:
	response, err := client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:  prompt,
				MinResults: minResults,
			},
		},
	}, 600*time.Second, os_client.REP_IO)
	if err != nil {
		goto retry
	}

	votesA := 0
	votesB := 0
	resultsProcessed := 0
	for _, choice := range response.GetCompletionResponse[0].Choices {
		_ = os.WriteFile("/tmp/final-report-vote.txt", []byte(choice), 0644)
		err := tools.ParseJSON(choice, func(s string) error {
			return json.Unmarshal([]byte(s), &parsedResponse)
		})
		if err != nil {
			continue
		}

		if parsedResponse.BestReport == "A" {
			votesA++
			resultsProcessed++
		} else if parsedResponse.BestReport == "B" {
			votesB++
			resultsProcessed++
		}
	}

	if resultsProcessed < 3 {
		minResults++
		goto retry
	}

	return votesA > votesB, nil
}

func codeblock(s string) string {
	return fmt.Sprintf("```\n%s\n```", s)
}
