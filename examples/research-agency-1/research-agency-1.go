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
	"github.com/logrusorgru/aurora"
	"os"
	"strings"
	"sync"
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
	finalReportsSink := make(chan string)
	finalReportsStream := make(chan string, 1024)
	agentState.FinalReportChannel = finalReportsSink
	go agentState.ToTPipeline()

	go func() {
		reports := make([]string, 0)
		for {
			select {
			case report := <-finalReportsSink:
				reports = append(reports, report)
				serialized, err := json.Marshal(reports)
				if err != nil {
					continue
				}

				_ = os.WriteFile("/tmp/final-reports.json", serialized, 0644)
				finalReportsStream <- report
			}
		}
	}()

	storedReports := make([]string, 0)
	storedReportsData, err := os.ReadFile("/tmp/final-reports.json")
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to read stored reports")
	}
	err = json.Unmarshal(storedReportsData, &storedReports)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to unmarshal stored reports")
	}
	for _, report := range storedReports {
		finalReportsStream <- report
	}

	finalReports := make([]string, 0)
	for finalReport := range finalReportsStream {
		fmt.Println(finalReport)
		collectedUpdatedReports := make([]string, 0)
		finalReports = append(finalReports, finalReport)
		if len(finalReports) > 2 {
			ratings := make(map[int]int)
			wg := sync.WaitGroup{}
			lock := sync.RWMutex{}
			for idxA, reportA := range finalReports {
				for idxB, reportB := range finalReports {
					if idxA == idxB {
						continue
					}
					if reportA == reportB {
						continue
					}

					compareReports := func(idxA, idxB int, reportA, reportB string) {
						defer wg.Done()
						yes, err := isReportABetter(client, finalReportsStream, agentState.InputVariables["goal"].(string), reportA, reportB)
						if err != nil {
							return
						}
						if yes {
							lock.Lock()
							ratings[idxA]++
							lock.Unlock()
							//ratings[idxB]--
						}
					}

					wg.Add(2)
					go compareReports(idxA, idxB, reportA, reportB)
					go compareReports(idxB, idxA, reportB, reportA)
				}
			}
			wg.Wait()

			fmt.Printf("ratings: %v\n", ratings)

			// print reports from worth to best
			printReports(ratings, finalReports)
			// dropping all current reports, leaving only those which scored
			newFinalReports := make([]string, 0)
			for idx, _ := range ratings {
				newFinalReports = append(newFinalReports, finalReports[idx])
			}
			finalReports = newFinalReports

			go func(reports []string) {
				for _, report := range reports {
					finalReportsStream <- report
				}
			}(collectedUpdatedReports)
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
	info := fmt.Sprintf("Total reports: %d\n", len(reports)) + sw
	_ = os.WriteFile("reports-table.txt", []byte(info), 0644)
	fmt.Println(info)
}

func isReportABetter(client *os_client.AgentOSClient, updatedReports chan string, goal string, a string, b string) (bool, error) {
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
    "best-report": "<A|B>",
}
%s
### Assistant: `
	// "updated-report": "full text of the updated and expanded report has been revised,\nfree from the shortcomings previously found and with correct usage of\nnext line symbol"
	type modelResponse struct {
		Thoughts   string `json:"thoughts"`
		BestReport string `json:"best-report"`
		// UpdatedReport string `json:"updated-report"`
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
		choice = strings.ReplaceAll(choice, "\",\n}", "\"\n}")
		err := tools.ParseJSON(choice, func(s string) error {
			return json.Unmarshal([]byte(s), &parsedResponse)
		})
		_ = os.WriteFile("/tmp/final-report-vote.txt", []byte(fmt.Sprintf(`
Collected updated reports: %d
Current choice is (parse error = %v):
%s
`, len(updatedReports), err, choice)), 0644)
		if err != nil {
			continue
		}

		if parsedResponse.BestReport == "A" {
			votesA++
			resultsProcessed++
		} else if parsedResponse.BestReport == "B" {
			votesB++
			resultsProcessed++
		} else if parsedResponse.BestReport == "Neither" || parsedResponse.BestReport == "A, B" || parsedResponse.BestReport == "A,B" {
			if len(a) > len(b) {
				votesA++
				resultsProcessed++
			} else {
				votesB++
				resultsProcessed++
			}
		}

		if parsedResponse.BestReport == "A" || parsedResponse.BestReport == "B" {
			//updatedReport := parsedResponse.UpdatedReport
			// let's try to get updated report now
			go generateUpdatedReport(client, prompt, choice, updatedReports)
		}
	}

	if resultsProcessed < 3 {
		minResults++
		goto retry
	}

	return votesA > votesB, nil
}

func generateUpdatedReport(client *os_client.AgentOSClient, prompt string, choice string, updatedReports chan string) {
	updatedReportQuery := fmt.Sprintf(`%s%s
### User: Provide full text of the updated and expanded report has been revised in markdown format.

### Assistant: `, prompt, choice)

retryUpdatedReport:
	updatedReportResponse, err := client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:  updatedReportQuery,
				MinResults: 1,
			},
		},
	}, 600*time.Second, os_client.REP_IO)
	if err != nil {
		time.Sleep(100 * time.Millisecond)
		goto retryUpdatedReport
	}

	updatedReport := updatedReportResponse.GetCompletionResponse[0].Choices[0]
	if updatedReport != "" && len(updatedReport) > 10 {
		fmt.Printf("Got updated report: %s\n", aurora.BrightGreen(updatedReport))
		updatedReports <- updatedReport
	}
}

func codeblock(s string) string {
	return fmt.Sprintf("```\n%s\n```", s)
}
