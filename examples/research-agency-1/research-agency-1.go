package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"github.com/d0rc/agent-os/utils"
	"github.com/logrusorgru/aurora"
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

var finalReportsPath = flag.String("final-reports-path", "/tmp/final-reports.json", "path to final reports storage")
var startAgency = flag.Bool("start-agency", true, "start agency")
var agentOSUrl = flag.String("agent-os-url", "http://127.0.0.1:9000", "agent-os endpoint")

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting research-agency-1")

	agencySettings, err := agency.ParseAgency(agencyYaml)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to parse agency")
	}

	lg.Info().Interface("agencySettings", agencySettings).Msg("parsed agency")

	client := os_client.NewAgentOSClient(*agentOSUrl)
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
	if *startAgency {
		go agentState.SoTPipeline(3, 80, 80)
	}

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

				_ = os.WriteFile(*finalReportsPath, serialized, 0644)
				finalReportsStream <- report
			}
		}
	}()

	storedReports := make([]string, 0)
	storedReportsData, err := os.ReadFile(*finalReportsPath)
	if err != nil {
		lg.Error().Err(err).Msg("failed to read stored reports")
	} else {
		err = json.Unmarshal(storedReportsData, &storedReports)
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to unmarshal stored reports")
		}
		for _, report := range removeDuplicates(storedReports) {
			finalReportsStream <- report
		}
	}

	maxHims := 10
	outputChannel := make(chan string)
	initialHim := &HIM{
		Id:                       fmt.Sprintf("him%03d", 0),
		InputFinalReportsStream:  finalReportsStream,
		OutputFinalReportsStream: outputChannel,
		ContextGoal:              agentState.InputVariables["goal"].(string),
		Client:                   client,
	}
	hims := []*HIM{initialHim}

	go func(him *HIM) {
		him.finalReportsMaker()
	}(initialHim)

	for {
		if len(hims) < maxHims {
			allBusy := true
			for _, him := range hims {
				if !him.Busy {
					allBusy = false
					break
				}
			}

			if allBusy {
				idx := len(hims)
				him := &HIM{
					Id:                       fmt.Sprintf("him%03d", idx),
					InputFinalReportsStream:  finalReportsStream,
					OutputFinalReportsStream: outputChannel,
					ContextGoal:              agentState.InputVariables["goal"].(string),
					Client:                   client,
				}

				go func(him *HIM) {
					him.finalReportsMaker()
				}(him)

				hims = append(hims, him)
			}
		} else {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	for output := range outputChannel {
		fmt.Println("OUTPUT: ", output)
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}

type HIM struct {
	Id                       string
	InputFinalReportsStream  chan string
	OutputFinalReportsStream chan string
	ContextGoal              string
	Client                   *os_client.AgentOSClient
	CollectionCycle          int
	ComputeCycles            int
	CycleBreaks              int
	Busy                     bool
}

func (him *HIM) Printf(sfmt string, args ...interface{}) {
	fmt.Printf(fmt.Sprintf("[%s] ", aurora.BrightYellow(him.Id))+sfmt, args...)
}

func (him *HIM) finalReportsMaker() {
	finalReports := make([]string, 0)
	for finalReport := range him.InputFinalReportsStream {
		him.CollectionCycle++
		finalReport = strings.TrimSpace(finalReport)
		finalReports = removeDuplicates(append(finalReports, finalReport))

		cycle := 0
		initialNumberOfReports := len(finalReports)
		for len(finalReports) > 2 {
			him.Busy = true
			finalReports = shuffle(finalReports)

			cycle++
			him.ComputeCycles++
			if cycle > 10 && len(finalReports) <= initialNumberOfReports {
				him.CycleBreaks++
				break
			}
			him.Printf("Running cycle %d of #%d over %d reports, started with %d reports.\n",
				cycle,
				him.CollectionCycle,
				len(finalReports),
				initialNumberOfReports)
			//newFinalReports, ratings := naiveComparator(agentState, finalReports, client, againCounter)

			var chunks = make([][]string, 0)
			chunkSize := 2

			finalReports = shuffle(finalReports)
			for i := 0; i < len(finalReports); i += chunkSize {
				end := i + chunkSize
				if end > len(finalReports) {
					end = len(finalReports)
				}
				chunks = append(chunks, finalReports[i:end])
			}

			chunksProcessingResults := make([]string, 0)
			chunksProcessingResultsLock := sync.RWMutex{}
			agentGoal := him.ContextGoal
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
					defer wg.Done()
					reportsAreSame, _ := areReportsEqual(him.Client, agentGoal, chunkReports[0], chunkReports[1])
					if reportsAreSame {
						him.Printf("Got equal reports for chunk %d\n", chunkIdx)
						chunksProcessingResultsLock.Lock()
						chunksProcessingResults = append(chunksProcessingResults, chunkReports[0])
						chunksProcessingResultsLock.Unlock()
						return
					}

					merged := him.generateUpdatedReport(agentGoal, chunkReports[0], chunkReports[1])
					if len(merged) == 0 {
						merged = append(merged, chunkReports...)
					}

					chunksProcessingResultsLock.Lock()
					chunksProcessingResults = append(chunksProcessingResults, merged...)
					chunksProcessingResultsLock.Unlock()
				}(chunkIdx, chunkReports, &chunksProcessingResults, &chunksProcessingResultsLock)
			}
			wg.Wait()

			if len(chunksProcessingResults) > 0 {
				finalReports = removeDuplicates(chunksProcessingResults)
			}

			him.Printf("Got %d reports, after merge phase\n", len(finalReports))

			newFinalReports, ratings := him.naiveComparator(finalReports)
			newFinalReports = removeDuplicates(newFinalReports)

			him.Printf("Got %d reports, after compare phase\n", len(newFinalReports))

			// print reports from worth to best
			him.printReports(ratings, finalReports, him.InputFinalReportsStream)
			if len(newFinalReports) > 0 {
				finalReports = newFinalReports
			}
		}
		him.Busy = false
	}
}

func areReportsEqual(client *os_client.AgentOSClient, goal, a, b string) (bool, error) {
	prompt := `### Instruction:
You are Report Comparing AI. You have to pick the best report for the primary goal.

Primary goal:
%s

Your task is to compare following two reports:
Report A:
%s

Report B:
%s

Please help to choose a report for further processing.
Are these reports the same?
Provide response in the following JSON format:

%s
{
    "thoughts": "thoughts text, discussing which report is more comprehensive and better aligns with the primary goal",
    "reports-are-equal": "<yes|no>",
}
%s

### Assistant: 
%s
`
	type yesNoResponse struct {
		Thoughts        string `json:"thoughts"`
		ReportsAreEqual string `json:"reports-are-equal"`
	}
	parsedResponse := yesNoResponse{}
	prompt = fmt.Sprintf(prompt, tools.CodeBlock(goal), tools.CodeBlock(a), tools.CodeBlock(b), "```json", "```", "```json")
	minResults := 3
	resultsToRequest := 0
retry:
	resultsToRequest += minResults
	response, err := client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: tools.Replicate(cmds.GetCompletionRequest{
			RawPrompt:   prompt,
			MinResults:  resultsToRequest,
			Temperature: 0.1,
		}, minResults),
	}, 600*time.Second, os_client.REP_Default)
	if err != nil {
		fmt.Printf("Error getting response, going to retry")
		goto retry
	}

	yesCounter := 0
	for _, choice := range tools.FlattenChoices(response.GetCompletionResponse) {
		err := tools.ParseJSON(choice, func(s string) error {
			return json.Unmarshal([]byte(s), &parsedResponse)
		})
		if err == nil {
			if parsedResponse.ReportsAreEqual == "yes" {
				yesCounter++
			}
		}
	}

	if yesCounter >= 2 {
		return true, nil
	}

	return false, nil
}

func shuffle(reports []string) []string {
	for i := len(reports) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		reports[i], reports[j] = reports[j], reports[i]
	}
	return reports
}

func (him *HIM) naiveComparator(finalReports []string) ([]string, map[int]int) {
	newFinalReports := make([]string, 0)
	ratings := make(map[int]int)
	wg := sync.WaitGroup{}
	lock := sync.RWMutex{}
	agentGoal := him.ContextGoal
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
					yes, err := him.isReportABetter(agentGoal, reportA, reportB)
					if err != nil {
						return
					}
					if yes {
						lock.Lock()
						ratings[idxA]++
						lock.Unlock()
					}
				}

				wg.Add(2)
				go compareReports(idxA, idxB, reportA, reportB)
				go compareReports(idxB, idxA, reportB, reportA)
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

func (him *HIM) printReports(ratings map[int]int, reports []string, reportsStream chan string) {
	maxVal := ratings[0]
	maxIdx := 0
	for idx, val := range ratings {
		if val > maxVal {
			maxVal = val
			maxIdx = idx
		}
	}
	// create a string writer
	sw := fmt.Sprintf(`
Collection cycle: %d, compute cycles: %d, cycle breaks: %d, best of %d report, has score of %d, ratings map is: %v:
%s
`, him.CollectionCycle, him.ComputeCycles, him.CycleBreaks, len(reports), maxVal, ratings, tools.CodeBlock(reports[maxIdx]))
	info := fmt.Sprintf("Total reports: %d, reports selected: %d, final-reports-len: %d, queue len: %d\n",
		atomic.LoadUint64(&reportsProcessed),
		len(ratings),
		len(reports),
		len(reportsStream))
	_ = os.WriteFile(fmt.Sprintf("reports-table-%s.txt", him.Id), []byte(info+sw), 0644)
}

type modelResponse struct {
	//Thoughts   string `json:"thoughts"`
	BestReport string `json:"best-report"`
	// UpdatedReport string `json:"updated-report"`
}

var errCounter uint64 = 0

func (him *HIM) isReportABetter(goal string, a string, b string) (bool, error) {
	prompt := tools.NewChatPrompt().
		AddSystem(fmt.Sprintf(`You are Report Comparing AI. You have to pick the best report for the primary goal.

Primary goal:
%s

Your task is to compare following two reports:
Report A:
%s

Report B:
%s

Please help to choose a report for further processing.
Which of the reports is more comprehensive and better aligns with the primary goal?
Provide response in the following JSON format:

%s
{
    "thoughts": "thoughts text, discussing which report is more comprehensive and better aligns with the primary goal",
    "best-report": "<A|both|B>",
}
%s`, tools.CodeBlock(goal), tools.CodeBlock(a), tools.CodeBlock(b), "```json", "```"))
	/*prompt := `### Instruction:
	You are Report Comparing AI. You have to pick the best report for the primary goal.

	Primary goal:
	%s

	Your task is to compare following two reports:
	Report A:
	%s

	Report B:
	%s

	Please help to choose a report for further processing.
	Which of the reports is more comprehensive and better aligns with the primary goal?
	Provide response in the following JSON format:

	%s
	{
	    "thoughts": "thoughts text, discussing which report is more comprehensive and better aligns with the primary goal",
	    "best-report": "<A|both|B>",
	}
	%s
	### Assistant:
	%s
	`
		prompt = fmt.Sprintf(prompt, tools.CodeBlock(goal), tools.CodeBlock(a), tools.CodeBlock(b), "```json", "```", "```json")*/
	// "updated-report": "full text of the updated and expanded report has been revised,\nfree from the shortcomings previously found and with correct usage of\nnext line symbol"
	parsedResponse := modelResponse{}

	minResults := 3
	resultsToRequest := 0
retry:
	resultsToRequest += minResults
	response, err := him.Client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: tools.Replicate(cmds.GetCompletionRequest{
			RawPrompt:   prompt.DefString(),
			MinResults:  resultsToRequest,
			Temperature: 0.9,
		}, minResults),
	}, 600*time.Second, os_client.REP_Default)
	if err != nil {
		him.Printf("Error getting response, going to retry")
		goto retry
	}

	votesA := 0
	votesB := 0
	resultsProcessed := 0
	//goodChoice := ""
	choices := tools.FlattenChoices(response.GetCompletionResponse)
	if len(choices) > resultsToRequest {
		resultsToRequest = len(choices) + 1
	}
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
			if len(a) < len(b) {
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
		} else {
			if len(a) < len(b) {
				votesA++
				resultsProcessed++
			} else {
				votesB++
				resultsProcessed++
			}
		}
	}

	if resultsProcessed < minResults {
		resultsToRequest++
		him.Printf("Got not enough results (%d/%d), going to retry\n", resultsProcessed,
			len(response.GetCompletionResponse[0].Choices))
		if len(response.GetCompletionResponse[0].Choices) > 15 {
			return false, fmt.Errorf("error getting parsable response")
		}
		goto retry
	}

	him.Printf("Got enough results (%d/%d) [%d vs %d]\n",
		resultsProcessed,
		len(response.GetCompletionResponse),
		votesA, votesB)

	return votesA > votesB, nil
}

func (him *HIM) generateUpdatedReport(goal, a, b string) []string {
	updatedReportQuery := tools.NewChatPrompt().
		AddSystem(fmt.Sprintf(`You are Report Merging AI. Your goal is to collect the information relevant to the primary goal.
Primary goal:
%s

Your task is to compare and merge if appropriate these drafts:

%s
%s

Please focus only on the facts and disregard any secondary or ancillary comments and discussions that the field agents have included in the drafts.
Re-structure the draft above to make it easy to read and comprehend, don't miss facts or entities in updated draft.
`,
			tools.CodeBlock(goal),
			tools.CodeBlock(a),
			tools.CodeBlock(b))).DefString()

	minResults := 0
retryUpdatedReport:
	minResults++
	updatedReportResponse, err := him.Client.RunRequest(&cmds.ClientRequest{
		ProcessName: "final-reports-processor",
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:   updatedReportQuery,
				MinResults:  minResults,
				Temperature: 0.9,
			},
		},
	}, 600*time.Second, os_client.REP_Default)
	if err != nil {
		time.Sleep(100 * time.Millisecond)
		him.Printf("Error getting response for updated report, going to retry")
		goto retryUpdatedReport
	}

	choices := tools.FlattenChoices(updatedReportResponse.GetCompletionResponse)
	if len(choices) > minResults {
		minResults = len(choices) + 1
		goto retryUpdatedReport
	}

	for _, updatedReport := range choices {
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
			allOptions := make([]string, 0)
			for _, reportA := range reportsList {
				reportAId := engines.GenerateMessageId(reportA)
				for _, reportB := range reportsList {
					reportBId := engines.GenerateMessageId(reportB)
					if reportAId == reportBId {
						continue
					}

					isBetter, err := him.isReportABetter(goal, reportA, reportB)
					if err != nil {
						continue
					}
					if isBetter {
						ratings[reportAId]++
						allOptions = append(allOptions, reportA)
					}
				}
			}

			// remove all duplicates
			results := removeDuplicates(allOptions)
			if len(results) <= 2 {
				return results
			}
		}
	}

	return []string{a, b}
}

func removeDuplicates(options []string) []string {
	results := make(map[string]struct{})
	resultsSlice := make([]string, 0)
	for _, option := range options {
		option = strings.TrimSpace(option)
		if _, ok := results[option]; !ok {
			results[option] = struct{}{}
			resultsSlice = append(resultsSlice, option)
		}
	}

	return resultsSlice
}

func isStringContains(a string, b []string) bool {
	for _, c := range b {
		if strings.Contains(a, c) {
			return true
		}
	}
	return false
}
