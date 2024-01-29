package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/stdlib/generics"
	"github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/d0rc/agent-os/syslib/utils"
	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"github.com/ulikunitz/xz"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var termUi = false

var reportsProcessed = uint64(0)

var finalReportsPath = flag.String("final-reports-path", "/tmp/final-reports.json", "path to final reports storage")
var startAgency = flag.Bool("start-agency", false, "start agency")
var agentOSUrl = flag.String("agent-os-url", "http://127.0.0.1:9000", "agent-os endpoint")
var config = flag.String("agency-config", "agency.yaml", "path to agency config")
var himHeads = flag.Int("him-heads", 1, "number of HIM-heads")
var primaryAgentThreads = flag.Int("primary-agent-threads", 120, "number of threads for primary agent")

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting research-agency-1")

	agencyYaml, err := os.ReadFile(*config)
	if err != nil {
		lg.Fatal().Err(err).Msgf("failed to read agency config, path = `%s`", *config)
	}

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
		go agentState.SoTPipeline(3, *primaryAgentThreads, *primaryAgentThreads)
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
		fmt.Printf("Got %d stored final reports\n", len(storedReports))
		for _, report := range tools.DropDuplicates(storedReports) {
			finalReportsStream <- report
		}
	}

	maxHims := *himHeads
	outputChannel := make(chan string)
	if maxHims > 0 {
		statsChannel := make(chan *HIMStats)

		go func(statsChannel chan *HIMStats) {
			statistics := make(map[string]*HIMStats)
			for stat := range statsChannel {
				statistics[stat.Id] = stat

				writeTable("him-stats.txt", statistics)
			}
		}(statsChannel)

		initialHim := &HIM{
			Id:                       fmt.Sprintf("him%03d", 0),
			InputFinalReportsStream:  finalReportsStream,
			OutputFinalReportsStream: outputChannel,
			ContextGoal:              agentState.InputVariables["goal"].(string),
			Client:                   client,
			Reports:                  statsChannel,
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
						Reports:                  statsChannel,
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
	Reports                  chan *HIMStats
}

func (him *HIM) Printf(sfmt string, args ...interface{}) {
	fmt.Printf(fmt.Sprintf("[%s] ", aurora.BrightYellow(him.Id))+sfmt, args...)
}

func (him *HIM) finalReportsMaker() {
	finalReports := make([]string, 0)
	for finalReport := range him.InputFinalReportsStream {
		him.CollectionCycle++
		finalReport = strings.TrimSpace(finalReport)
		finalReports = tools.DropDuplicates(append(finalReports, finalReport))

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
				finalReports = tools.DropDuplicates(chunksProcessingResults)
			}

			him.Printf("Got %d reports, after merge phase\n", len(finalReports))

			newFinalReports, ratings := him.naiveComparator(finalReports)
			newFinalReports = tools.DropDuplicates(newFinalReports)

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
	yesCounter := uint64(0)
	locker := sync.RWMutex{}
	err := generics.CreateSimplePipeline(client, "him-equality-test").
		WithSystemMessage(`You are Report Comparing AI. You have to pick the best report for the primary goal.

Primary goal:
{{goal}}

Your task is to compare following two reports:
Report A:
{{repA}}

Report B:
{{repB}}

Please help to choose a report for further processing.
Are these reports the same?`).
		WithVar("goal", tools.CodeBlock(goal)).
		WithVar("repA", tools.CodeBlock(a)).
		WithVar("repB", tools.CodeBlock(b)).
		WithResponseField("thoughts", "self-thoughts, discussing which report is more comprehensive and better aligns with the primary goal").
		WithResponseField("reports-are-equal", "<yes|no>").
		WithResultsProcessor("reports-are-equal", func(choice string) (generics.ResultProcessingOutcome, error) {
			if choice == "yes" {
				locker.Lock()
				yesCounter++
				locker.Unlock()
				return generics.RPOProcessed, nil
			} else if choice == "no" {
				return generics.RPOProcessed, nil
			}

			return generics.RPOFailed, fmt.Errorf("invalid choice")
		}).
		Run(os_client.REP_IO)

	if err != nil {
		return false, err
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

type HIMStats struct {
	Id              string
	CollectionCycle int
	ComputeCycles   int
	CycleBreaks     int
	PoolSize        int
	RatingsMap      string
	BestSize        int
	BestEntropy     float64
	AllSize         int
	AllEntropy      float64
	BestReportIdx   int
	AllReports      []string
	BestReportScore int
}

func writeTable(fname string, statistics map[string]*HIMStats) {
	buf := &strings.Builder{}
	tw := tablewriter.NewWriter(buf)
	tw.SetHeader([]string{"Id",
		"CollectionCycle",
		"ComputeCycles",
		"CycleBreaks",
		"PoolSize",
		"RatingsMap",
		"BestSize",
		"BestEntropy",
		"AllSize",
		"AllEntropy",
	})
	for _, stat := range statistics {
		tw.Append([]string{stat.Id,
			strconv.Itoa(stat.CollectionCycle),
			strconv.Itoa(stat.ComputeCycles),
			strconv.Itoa(stat.CycleBreaks),
			strconv.Itoa(stat.PoolSize),
			stat.RatingsMap,
			strconv.Itoa(stat.BestSize),
			strconv.FormatFloat(stat.BestEntropy, 'f', 2, 64),
			strconv.Itoa(stat.AllSize),
			strconv.FormatFloat(stat.AllEntropy, 'f', 2, 64),
		})
	}

	tw.Render()

	_ = os.WriteFile(fname, []byte(buf.String()), 0644)
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

	stats, _ := getSizeAndEntropy(maxIdx, reports)

	// create a string writer
	sep := "\n==================================================================================================\n"
	allReports := strings.Join(reports, sep)
	sw := fmt.Sprintf(`All reports:
%s
%s

[HIM] Collection cycle: %d, compute cycles: %d, cycle breaks: %d, best of %d report, has score of %d, ratings map is: %v.
[HIM] BestSize: %d, BestEntropy: %2.4f(bytes/symbol), AllSize: %d, AllEntropy: %2.4f(bytes/symbol).
%s
`, allReports, sep, him.CollectionCycle, him.ComputeCycles, him.CycleBreaks, len(reports), maxVal, ratings,
		stats.BestSize, stats.BestEntropy, stats.AllSize, stats.AllEntropy,
		tools.CodeBlock(reports[maxIdx]))

	himStats := &HIMStats{
		Id:              him.Id,
		CollectionCycle: him.CollectionCycle,
		ComputeCycles:   him.ComputeCycles,
		CycleBreaks:     him.CycleBreaks,
		PoolSize:        len(reports),
		RatingsMap:      fmt.Sprintf("%v", ratings),
		BestSize:        stats.BestSize,
		BestEntropy:     stats.BestEntropy,
		AllSize:         stats.AllSize,
		AllEntropy:      stats.AllEntropy,
		BestReportIdx:   maxIdx,
		BestReportScore: maxVal,
		AllReports:      reports,
	}

	if him.Reports != nil {
		him.Reports <- himStats
	}

	_ = os.WriteFile(fmt.Sprintf("reports-table-%s.txt", him.Id), []byte(sw), 0644)
}

func getSizeAndEntropy(best int, reports []string) (*struct {
	BestSize    int
	BestEntropy float64
	AllSize     int
	AllEntropy  float64
}, error) {
	bestSize := 0
	bestEntropy := 0.0
	allSize := 0
	allEntropy := 0.0

	allReportsBuilder := strings.Builder{}
	for i, report := range reports {
		allSize = allSize + len(report)
		if i == best {
			bestSize = len(report)
		}
		allReportsBuilder.WriteString(report)
	}

	xzCompressedBest, err := xzCompress(reports[best])
	if err != nil {
		return nil, err
	}
	xzCompressedAll, err := xzCompress(allReportsBuilder.String())
	if err != nil {
		return nil, nil
	}

	bestEntropy = float64(len(xzCompressedBest)) / float64(bestSize)
	allEntropy = float64(len(xzCompressedAll)) / float64(allSize)

	return &struct {
		BestSize    int
		BestEntropy float64
		AllSize     int
		AllEntropy  float64
	}{
		BestSize:    bestSize,
		BestEntropy: bestEntropy,
		AllSize:     allSize,
		AllEntropy:  allEntropy,
	}, nil
}

var lzmaDictCapExps = []uint{18, 20, 21, 22, 22, 23, 23, 24, 25, 26}

func xzCompress(s string) ([]byte, error) {
	var buf bytes.Buffer

	// Configure the writer with maximum compression level
	writer, err := xz.WriterConfig{
		DictCap: 1 << lzmaDictCapExps[9],
	}.NewWriter(&buf)

	if err != nil {
		return nil, err
	}

	_, err = writer.Write([]byte(s))
	if err != nil {
		writer.Close()
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type modelResponse struct {
	//Thoughts   string `json:"thoughts"`
	BestReport string `json:"best-report"`
	// UpdatedReport string `json:"updated-report"`
}

var errCounter uint64 = 0

func (him *HIM) isReportABetter(goal string, a string, b string) (bool, error) {
	votesA := uint64(0)
	votesB := uint64(0)
	resultsProcessed := uint64(0)
	resultsAttempted := uint64(0)

	err := generics.CreateSimplePipeline(him.Client, "him-is-better-test").
		WithSystemMessage(`You are Report Comparing AI. You have to pick the best report for the primary goal.

Primary goal:
{{goal}}

Your task is to compare the following two reports:
Report A:
{{repA}}

Report B:
{{repB}}

Please help me choose a report for further processing.
Which of the reports is more comprehensive and better aligns with the primary goal?`).
		WithVar("goal", goal).
		WithVar("repA", a).
		WithVar("repB", b).
		WithResponseField("thoughts", "self-thoughts, discussing which report is more comprehensive and better aligns with the primary goal").
		WithResponseField("best-report", "<A|both|B>").
		WithFullResultProcessor(func(choice string) (generics.ResultProcessingOutcome, error) {
			atomic.AddUint64(&resultsAttempted, 1)
			choice = strings.ReplaceAll(choice, "\",\n}", "\"\n}")

			// Define regular expressions for matching report types
			reportARegex := regexp.MustCompile(`"best-report": "(A|Report A|ReportA|<A>|Report A with the addendum .*)`)
			reportBRegex := regexp.MustCompile(`"best-report": "(B|Report B|ReportB|<B>)`)
			reportBothRegex := regexp.MustCompile(`"best-report": "(A and B|A,B|<A,B>|<A, B>|<BOTH A and B>|both)"`)

			parsedResponse := modelResponse{}
			// Process the choice
			var err error
			switch {
			case reportBothRegex.MatchString(choice):
				parsedResponse.BestReport = "X"
			case reportARegex.MatchString(choice):
				parsedResponse.BestReport = "A"
			case reportBRegex.MatchString(choice):
				parsedResponse.BestReport = "B"
			default:
				err = json.Unmarshal([]byte(choice), &parsedResponse)
				if err != nil {
					// Handle error and continue
					atomic.AddUint64(&errCounter, 1)
					_ = os.WriteFile("/tmp/final-report-vote.txt", []byte(fmt.Sprintf(`
Current choice is (total errors = %d, parse error = %v):
%s
`, errCounter, err, choice)), 0644)
					return generics.RPOFailed, err
				}
			}

			// Update votes based on parsed response
			switch {
			case strings.HasPrefix(parsedResponse.BestReport, "A"):
				atomic.AddUint64(&votesA, 1)
			case strings.HasPrefix(parsedResponse.BestReport, "B"):
				atomic.AddUint64(&votesB, 1)
			default:
				if len(a) < len(b) {
					atomic.AddUint64(&votesA, 1)
				} else {
					atomic.AddUint64(&votesB, 1)
				}
			}

			atomic.AddUint64(&resultsProcessed, 1)
			return generics.RPOProcessed, nil
		}).Run(os_client.REP_IO)

	him.Printf("Got enough results (%d/%d) [%d vs %d] (err=%v)\n",
		resultsProcessed,
		resultsAttempted,
		votesA, votesB,
		err)

	return votesA > votesB, err
}

func (him *HIM) generateUpdatedReport(goal, a, b string) []string {
	updatedReportQuery := tools.NewChatPrompt().
		AddSystem(fmt.Sprintf(`You are Report Merging AI. Your goal is to collect the information relevant to the primary goal.
Primary goal:
%s

Your task is to compare these drafts and merge them if necessary:

%s
%s

Please focus only on the facts and disregard any secondary or ancillary comments and discussions that the field agents have included in the drafts.
Re-structure the draft above to make it easy to read and comprehend.  Don't miss or exclude any facts or anything else important.
`,
			tools.CodeBlock(goal),
			tools.CodeBlock(a),
			tools.CodeBlock(b))).DefString()

	minResults := 0
retryUpdatedReport:
	minResults++
	updatedReportResponse, err := him.Client.RunRequest(&cmds.ClientRequest{
		ProcessName: "him-merger",
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
			results := tools.DropDuplicates(allOptions)
			if len(results) <= 2 {
				return results
			}
		}
	}

	return []string{a, b}
}
