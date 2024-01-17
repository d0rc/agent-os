package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"github.com/d0rc/agent-os/utils"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed agency.yaml
var agencyYaml []byte

var termUi = false

var reportsProcessed = uint64(0)

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
		go newAgentState.SoTPipeline(1, 1, 1)

		return finalReportsStream
	}

	agentState.ForkCallback = spawningCallback
	finalReportsSink := make(chan string)
	finalReportsStream := make(chan string, 4096)
	agentState.FinalReportChannel = finalReportsSink
	//go agentState.ToTPipeline()
	//go agentState.SoTPipeline(3, 20, 20)

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
		lg.Error().Err(err).Msg("failed to read stored reports")
	} else {
		err = json.Unmarshal(storedReportsData, &storedReports)
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to unmarshal stored reports")
		}
		for _, report := range tools.DropDuplicates(storedReports) {
			finalReportsStream <- report
		}
	}

	finalReports := make([]string, 0)
	for finalReport := range finalReportsStream {
		finalReport = strings.TrimSpace(finalReport)
		finalReports = tools.DropDuplicates(append(finalReports, finalReport))

		if len(finalReports) > 2 {
			//newFinalReports, ratings := naiveComparator(agentState, finalReports, client, againCounter)

			startingLength := len(finalReports)
			for {
				// we need to split finalReports into chunks of chunkSize
				var chunks [][]string = make([][]string, 0)
				chunkSize := 2

				finalReports = shuffle(finalReports)
				for i := 0; i < len(finalReports); i += chunkSize {
					end := i + chunkSize
					if end > len(finalReports) {
						end = len(finalReports)
					}
					chunks = append(chunks, finalReports[i:end])
				}

				tools.AppendFile("hdr.log", fmt.Sprintf("Final reports: %d, chunks: %d, chunk size: %d\n",
					len(finalReports),
					len(chunks),
					chunkSize))

				chunksProcessingResults := make([]string, 0)
				chunksProcessingResultsLock := sync.RWMutex{}
				agentGoal := agentState.InputVariables["goal"].(string)
				wg := sync.WaitGroup{}
				for chunkIdx, chunkReports := range chunks {
					if len(chunkReports) != 2 {
						chunksProcessingResultsLock.Lock()
						chunksProcessingResults = append(chunksProcessingResults, chunkReports...)
						chunksProcessingResultsLock.Unlock()
						continue
					}
					wg.Add(1)
					go func(chunkIdx int, chunkReports []string, results *[]string, lock *sync.RWMutex) {
						merged := generateUpdatedReport(client, agentGoal, chunkReports[0], chunkReports[1])
						chunksProcessingResultsLock.Lock()
						chunksProcessingResults = append(chunksProcessingResults, merged...)
						chunksProcessingResultsLock.Unlock()
						tools.AppendFile("hdr.log",
							fmt.Sprintf("Processed chunk %d, output size is %d\n", chunkIdx, len(merged)))
						wg.Done()
					}(chunkIdx, chunkReports, &chunksProcessingResults, &chunksProcessingResultsLock)
				}
				wg.Wait()

				if len(chunksProcessingResults) > 0 {
					finalReports = tools.DropDuplicates(chunksProcessingResults)
				} else {
					tools.AppendFile("hdr.log", fmt.Sprintf("No results for chunks, output size is 0"))
				}

				newFinalReports, ratings := naiveComparator(agentState, finalReports, client, 0)
				newFinalReports = tools.DropDuplicates(newFinalReports)

				tools.AppendFile("hdr.log", fmt.Sprintf("ratings: %v\n", ratings))

				// print reports from worth to best
				printReports(ratings, finalReports, finalReportsStream)
				if len(newFinalReports) > 0 {
					finalReports = newFinalReports
				}
				// dropping all current reports, leaving only those which scored

				if len(finalReports) == startingLength {
					break
				}
				startingLength = len(finalReports)
			}
		}
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}

func shuffle(reports []string) []string {
	//rand.Seed(time.Now().UnixNano())
	for i := len(reports) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		reports[i], reports[j] = reports[j], reports[i]
	}
	return reports
}

func naiveComparator(agentState *agency.GeneralAgentInfo, finalReports []string, client *os_client.AgentOSClient, againCounter int) ([]string, map[int]int) {
	newFinalReports := make([]string, 0)
	ratings := make(map[int]int)
	wg := sync.WaitGroup{}
	lock := sync.RWMutex{}
	agentGoal := agentState.InputVariables["goal"].(string)
	for idxA, reportA := range finalReports {
		for idxB, reportB := range finalReports {
			if idxA == idxB {
				continue
			}
			if reportA == reportB {
				continue
			}

			wg.Add(1)
			go func(idxA, idxB int, reportA, reportB string) {
				defer wg.Done()

				compareReports := func(idxA, idxB int, reportA, reportB string) {
					defer wg.Done()
					yes, err := isReportABetter(client, agentGoal, reportA, reportB)
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
				/*if againCounter == 1 {
					updatedReport := generateUpdatedReport(client, nil, agentGoal, reportA, reportB)
					if updatedReport != "" {
						newFinalReports = append(newFinalReports, updatedReport)
					}
				}*/
				//updatedReport := generateUpdatedReport(client, nil, agentGoal, reportA, reportB)
				//if updatedReport != "" {
				//	newFinalReports = append(newFinalReports, updatedReport)
				//}
			}(idxA, idxB, reportA, reportB)
		}
	}
	wg.Wait()

	for idx, v := range ratings {
		if v > 0 {
			newFinalReports = append(newFinalReports, finalReports[idx])
		}
	}

	return newFinalReports, ratings
}

func printReports(ratings map[int]int, reports []string, reportsStream chan string) {
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
	info := fmt.Sprintf("Total reports: %d, reports selected: %d, final-reports-len: %d, queue len: %d\n",
		atomic.LoadUint64(&reportsProcessed),
		len(ratings),
		len(reports),
		len(reportsStream)) + sw
	_ = os.WriteFile("reports-table.txt", []byte(info), 0644)
	fmt.Println(info)
}

type modelResponse struct {
	//Thoughts   string `json:"thoughts"`
	BestReport string `json:"best-report"`
	// UpdatedReport string `json:"updated-report"`
}

var errCounter uint64 = 0

func isReportABetter(client *os_client.AgentOSClient, goal string, a string, b string) (bool, error) {
	prompt := `### Instruction:
You are Report Comparing AI. You have to pick the best report for the primary goal.

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
### Assistant: 
%s
`
	// "updated-report": "full text of the updated and expanded report has been revised,\nfree from the shortcomings previously found and with correct usage of\nnext line symbol"
	parsedResponse := modelResponse{}
	prompt = fmt.Sprintf(prompt, codeblock(goal), codeblock(a), codeblock(b), "```json", "```", "```json")
	minResults := 5
retry:
	response, err := client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: tools.Replicate(cmds.GetCompletionRequest{
			RawPrompt:   prompt,
			MinResults:  minResults,
			Temperature: 0.9,
		}, minResults),
	}, 600*time.Second, os_client.REP_Default)
	if err != nil {
		fmt.Printf("Error getting response, going to retry")
		goto retry
	}

	votesA := 0
	votesB := 0
	resultsProcessed := 0
	//goodChoice := ""
	for _, choice := range tools.FlattenChoices(response.GetCompletionResponse) {
		choice = strings.ReplaceAll(choice, "\",\n}", "\"\n}")
		if isStringContains(choice, []string{
			`"best-report": "A|B"`,
			`best-report": "A and B"`,
			`best-report": "A, B"`,
			`best-report": "<A|B>"`,
			`best-report": "<A,B>"`,
			`best-report": "<A, B>"`,
			`best-report": "<B|A>"`,
			`best-report": "<BOTH A and B>"`,
			`best-report": "both"`,
		}) {
			if len(a) > len(b) {
				votesA++
			} else {
				votesB++
			}
			resultsProcessed++
			continue
		} else if isStringContains(choice, []string{
			`"best-report": "A"`,
			`"best-report": "Report A"`,
			`"best-report": "ReportA"`,
			`"best-report": "<A>"`,
			`"best-report": "Report A with the addendum of having the detailed information about the restaurants as displayed in report B."`,
		}) {
			parsedResponse.BestReport = "A"
			//parsedResponse.Thoughts = ""
		} else if isStringContains(choice, []string{
			`"best-report": "B"`,
			`"best-report": "Report B"`,
			`"best-report": "ReportB"`,
			`"best-report": "<B>"`}) {
			parsedResponse.BestReport = "B"
			//parsedResponse.Thoughts = ""
		} else {
			err := tools.ParseJSON(choice, func(s string) error {
				return json.Unmarshal([]byte(s), &parsedResponse)
			})
			_ = os.WriteFile("/tmp/final-report-vote.txt", []byte(fmt.Sprintf(`
Current choice is (total errors = %d, parse error = %v):
%s
`, atomic.AddUint64(&errCounter, 1), err, choice)), 0644)
			if err != nil {
				continue
			}
		}

		if parsedResponse.BestReport == "A" || strings.HasPrefix(parsedResponse.BestReport, "A_") {
			votesA++
			resultsProcessed++
		} else if parsedResponse.BestReport == "B" || strings.HasPrefix(parsedResponse.BestReport, "B_") {
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
			//goodChoice = choice
		}
	}

	if resultsProcessed < 3 {
		minResults++
		fmt.Printf("Got not enough results (%d/%d), going to retry\n", resultsProcessed,
			len(response.GetCompletionResponse[0].Choices))
		if len(response.GetCompletionResponse[0].Choices) > 15 {
			return false, fmt.Errorf("error getting parsable response")
		}
		goto retry
	}

	fmt.Printf("Got enough results (%d/%d) [%d vs %d]\n",
		resultsProcessed,
		len(response.GetCompletionResponse),
		votesA, votesB)

	return votesA > votesB, nil
}

func generateUpdatedReport(client *os_client.AgentOSClient,
	goal, a, b string) []string {
	updatedReportQuery := fmt.Sprintf(`### Instruction:
You are Report Merging AI. Your goal is to collect the information relevant to the primary goal.
Primary goal:
%s

Your task is to compare following two reports:
Report A:
%s

Report B:
%s

### User: Summarize reports A and B into a single summary, mention any contradictions you find, use markdown format:

### Assistant:
%s`,
		codeblock(goal),
		codeblock(a),
		codeblock(b),
		"```json\n")

retryUpdatedReport:
	updatedReportResponse, err := client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:   updatedReportQuery,
				MinResults:  1,
				Temperature: 0.1,
			},
		},
	}, 600*time.Second, os_client.REP_Default)
	if err != nil {
		time.Sleep(100 * time.Millisecond)
		fmt.Printf("Error getting response for updated report, going to retry")
		goto retryUpdatedReport
	}

	updatedReport := updatedReportResponse.GetCompletionResponse[0].Choices[0]
	contentCheck := strings.ReplaceAll(updatedReport, " ", "")
	contentCheck = strings.ReplaceAll(contentCheck, "`", "")
	contentCheck = strings.ReplaceAll(contentCheck, "\n", "")
	contentCheck = strings.ReplaceAll(contentCheck, "\r", "")
	contentCheck = strings.ReplaceAll(contentCheck, "\t", "")
	lowerCasedReport := strings.ToLower(updatedReport)
	if updatedReport != "" && len(updatedReport) > 10 && len(contentCheck) > 10 &&
		!strings.Contains(lowerCasedReport, "report a") &&
		!strings.Contains(lowerCasedReport, "report b") &&
		!strings.Contains(lowerCasedReport, "reports a") {
		ratings := make(map[string]int)
		reportsList := append([]string{}, a, b, updatedReport)
		updatedReportId := engines.GenerateMessageId(updatedReport)
		for _, reportA := range reportsList {
			reportAId := engines.GenerateMessageId(reportA)
			for _, reportB := range reportsList {
				reportBId := engines.GenerateMessageId(reportB)
				if reportAId == reportBId {
					continue
				}
				if !(reportAId == updatedReportId || reportBId == updatedReportId) {
					continue
				}
				isBetter, err := isReportABetter(client, goal, reportA, reportB)
				if err != nil {
					continue
				}
				if isBetter {
					ratings[reportAId]++
				}
			}
		}

		// no let's pick to best and return those
		var maxKey1, maxKey2 string
		var maxValue1, maxValue2 int

		for key, value := range ratings {
			if value > maxValue1 {
				// Update second highest
				maxValue2 = maxValue1
				maxKey2 = maxKey1

				// Update highest
				maxValue1 = value
				maxKey1 = key
			} else if value > maxValue2 {
				// Update second highest
				maxValue2 = value
				maxKey2 = key
			}
		}
		// if any of maxKey1, or maxKey2 is empty, then we have no choice
		// but to return a or b instead of empty strings
		allOptions := []string{maxKey1, maxKey2, a, b}
		// remove all empty strings
		for i, option := range allOptions {
			if option == "" {
				allOptions = append(allOptions[:i], allOptions[i+1:]...)
			}
		}
		// remove all duplicates
		return tools.DropDuplicates(allOptions)

	}

	return []string{a, b}
}

func codeblock(s string) string {
	return fmt.Sprintf("```\n%s\n```", s)
}

func isStringContains(a string, b []string) bool {
	for _, c := range b {
		if strings.Contains(a, c) {
			return true
		}
	}
	return false
}
