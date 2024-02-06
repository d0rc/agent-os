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
	"github.com/rs/zerolog"
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
var config = flag.String("agency-config", "../work2/agency.yaml", "path to agency config")
var himHeads = flag.Int("him-heads", 1, "number of HIM-heads")
var primaryAgentThreads = flag.Int("primary-agent-threads", 120, "number of threads for primary agent")
var primaryGrowthFactor = flag.Int("primary-growth-factor", 10, "number of ways for primary agent to try")

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
	startedAgents := make(map[string]chan string)
	agentsLock := sync.RWMutex{}
	spawningCallback = func(name, goal string) chan string {
		agentsLock.Lock()
		if ch, exists := startedAgents[name+goal]; exists {
			agentsLock.Unlock()
			return ch
		}
		finalReportsStream := make(chan string, 10)
		startedAgents[name+goal] = finalReportsStream
		agentsLock.Unlock()

		clonedSettings, err := agency.ParseAgency(agencyYaml)
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to parse agency")
		}

		clonedSettings[0].Agent.Name = name
		clonedSettings[0].Agent.PromptBased.Vars[agency.IV_GOAL] = goal
		newAgentState := agency.NewGeneralAgentState(client, "", clonedSettings[0])

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
		go agentState.SoTPipeline(*primaryGrowthFactor, *primaryAgentThreads, *primaryAgentThreads)
	}

	go func() {
		reports := make([]string, 0)
		for {
			select {
			case report := <-finalReportsSink:
				reports = tools.DropDuplicates(append(reports, report))
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
		maxThreads := make(chan struct{}, 32)
		for _, report := range tools.DropDuplicates(storedReports) {
			maxThreads <- struct{}{}
			go func(report string) {
				defer func() {
					<-maxThreads
				}()
				resultYes := checkIfReportHasPlaceholdersOrFillers(client, report, lg)
				if resultYes == true {
					return
				}

				resultYes = checkIfReportHasAnyDataInIt(client, report, lg)
				if resultYes == false {
					return
				}

				finalReportsStream <- report
			}(report)
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

func checkIfReportHasPlaceholdersOrFillers(client *os_client.AgentOSClient, report string, lg zerolog.Logger) bool {
	var resultYes bool = true
	err := generics.CreateSimplePipeline(client, "Quality Checker").
		WithProcessName("quality-checker").
		WithSystemMessage(`You are reports quality checker, your goal is to detect placeholders and fillers in the reports you are processing.

Current report:
{{report}}

`).
		WithVar("report", report).
		WithResponseField("contains-fillers", "<yes|no>").
		WithResultsProcessor(func(responseMap map[string]interface{}, choice string) error {
			if responseMap != nil {
				modelResponse, exists := responseMap["contains-fillers"]
				if exists {
					tmpResult, ok := modelResponse.(string)
					if ok {
						if tmpResult == "yes" {
							resultYes = true
							return nil
						}
						if tmpResult == "no" {
							resultYes = false
							return nil
						}
					}
				}
			}
			return fmt.Errorf("error getting result from model")
		}).
		WithMinParsableResults(1).
		Run(os_client.REP_IO)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to run pipeline")
	}
	return resultYes
}

func checkIfReportHasAnyDataInIt(client *os_client.AgentOSClient, report string, lg zerolog.Logger) bool {
	resultYes := false
	err := generics.CreateSimplePipeline(client, "Quality Checker").
		WithProcessName("quality-checker").
		WithSystemMessage(`You are reports quality checker, your goal is to detect if report you are processing contains any significant results.

Reports describing research process and contain no research results should be marked as "contains-data": "no".
`).
		//WithVar("report", tools.CodeBlock(report)).
		WithResponseField("thoughts", "reflect on research results presented in the report").
		WithResponseField("contains-data", "<yes|no>").
		WithUserMessage(fmt.Sprintf("Current report:\n%s\nEnd of current report.", tools.CodeBlock(report))).
		WithResultsProcessor(func(responseMap map[string]interface{}, choice string) error {
			if responseMap != nil {
				modelResponse, exists := responseMap["contains-data"]
				thoughts, thoughtsExists := responseMap["thoughts"]
				if thoughtsExists {
					thoughtsString, ok := thoughts.(string)
					if ok && thoughtsString == "discuss text provided by user" {
						return fmt.Errorf("broken logic")
					}
				}
				if exists {
					tmpResult, ok := modelResponse.(string)
					if ok {
						if tmpResult == "yes" {
							resultYes = true
							return nil
						}
						if tmpResult == "no" {
							resultYes = false
							return nil
						}
					}
				}
			}
			return fmt.Errorf("error getting result from model")
		}).
		WithMinParsableResults(1).
		Run(os_client.REP_IO)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to run pipeline")
	}
	return resultYes
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

			him.Printf("cycle %d, started with %d reports\n", cycle, len(finalReports))

			tmpFinalReports, mergeInstructions, err := superDeduplicate(him.Client, him.ContextGoal, finalReports, true)
			if err == nil {
				finalReports = tmpFinalReports
			}

			finalReports = shuffle(finalReports)

			finalReports = him.generateUpdatedReports(him.ContextGoal, finalReports, mergeInstructions)

			tmpFinalReports, _, err = superDeduplicate(him.Client, him.ContextGoal, finalReports, false)
			if err == nil {
				finalReports = tmpFinalReports
			}

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
			/*
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
					var err error
					finalReports, err = superDeduplicate(him.Client, agentGoal, tools.DropDuplicates(chunksProcessingResults))
					if err != nil {
						finalReports = tools.DropDuplicates(chunksProcessingResults)
					}
				}*/

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

func (him *HIM) generateUpdatedReports(goal string, reports []string, instructions map[string][]string) []string {
	outputs := make([]string, 0)
	lock := sync.RWMutex{}
	wg := sync.WaitGroup{}
	for idxA, reportA := range reports {
		for idxB, reportB := range reports {
			if idxA == idxB {
				continue
			}

			wg.Add(1)
			go func(goal, reportA, reportB string) {
				merged := him.generateUpdatedReport(goal, reportA, reportB, instructions[fmt.Sprintf("%s:%s", reportA, reportB)])
				//wg2 := sync.WaitGroup{}
				lock.Lock()
				outputs = append(outputs, merged...)
				lock.Unlock()

				wg.Done()
			}(goal, reportA, reportB)
		}
	}

	wg.Wait()

	return outputs
}

func superDeduplicate(client *os_client.AgentOSClient, goal string, reports []string, returnInstructions bool) ([]string, map[string][]string, error) {
	mergeInstructions := make(map[string][]string)
	deduplicated := make(map[string]string)
	for idxA, reportA := range reports {
		for idxB, reportB := range reports {
			if idxA == idxB {
				continue
			}

			reportsAreSame, instructions, err := areReportsEqual(client, goal, reportA, reportB, returnInstructions)
			if instructions != nil && len(instructions) > 0 {
				mergeInstructions[fmt.Sprintf("%s:%s", reportA, reportB)] = tools.DropDuplicates(append(mergeInstructions[reportA], instructions...))
			}
			if err != nil {
				return nil, nil, err
			}
			// let's check which report is in the map already...!

			_, repArecorded := deduplicated[reportA]
			_, repBrecorded := deduplicated[reportB]

			if reportsAreSame && (repArecorded || repBrecorded) {
				continue
			}

			if reportsAreSame {
				deduplicated[reportA] = reportA
			} else {
				deduplicated[reportA] = reportA
				deduplicated[reportB] = reportB
			}
		}
	}

	results := make([]string, 0, len(deduplicated))
	for _, rep := range deduplicated {
		results = append(results, rep)
	}

	return results, mergeInstructions, nil
}

func areReportsEqual(client *os_client.AgentOSClient, goal, a, b string, returnInstructions bool) (bool, []string, error) {
	mergeInstructions := make([]string, 0)
	yesCounter := uint64(0)
	locker := sync.RWMutex{}
	err := generics.CreateSimplePipeline(client, "him-equality-test").
		WithSystemMessage(`You are Report Comparing AI. You have to pick the best report for the primary goal.

Your task is to compare following two reports:
Report A:
{{repA}}

Report B:
{{repB}}

Please help to choose a report for further processing.
Are these reports interchangeable?`).
		WithVar("goal", tools.CodeBlock(goal)).
		WithVar("repA", tools.CodeBlock(a)).
		WithVar("repB", tools.CodeBlock(b)).
		WithResponseField("thoughts", "self-thoughts, discussing which report is more comprehensive and better aligns with the primary goal").
		WithResponseField("reports-are-interchangeable", "<yes|no>").
		ConditionalField(returnInstructions, func(sp *generics.SimplePipeline) *generics.SimplePipeline {
			return sp.WithResponseField("instructions", "instructions on merging these reports by re-writing into a single text and merging lists of similar entities, i.e. without any additional research")
		}).
		WithMinParsableResults(1).
		WithResultsProcessor(func(resp map[string]interface{}, choice string) error {
			if v, exists := resp["reports-are-interchangeable"]; exists {
				if vString, ok := v.(string); ok {
					if vString == "yes" {
						locker.Lock()
						yesCounter++
						locker.Unlock()
						_ = os.WriteFile("reports-are-equal.txt", []byte(choice), 0666)
						return nil
					}
					if vString == "no" {
						if instructions, ok := resp["instructions"].(string); ok && instructions != "" {
							mergeInstructions = append(mergeInstructions, instructions)
						}
						_ = os.WriteFile("reports-not-equal.txt", []byte(choice), 0666)
						return nil
					}
				}
			}

			_ = os.WriteFile("reports-unknown-equality.txt", []byte(choice), 0666)
			return fmt.Errorf("invalid choice")
		}).
		Run(os_client.REP_IO)

	if err != nil {
		return false, mergeInstructions, err
	}

	if yesCounter >= 1 {
		return true, nil, nil
	}

	return false, mergeInstructions, nil
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
	if reports == nil || len(reports) == 0 {
		return
	}
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

	if best >= len(reports) {
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
	BestReport string `json:"recommendation"`
	// UpdatedReport string `json:"updated-report"`
}

var errCounter uint64 = 0

func (him *HIM) isReportABetter(goal string, a string, b string) (bool, error) {
	votesA := uint64(0)
	votesB := uint64(0)
	resultsProcessed := uint64(0)
	resultsAttempted := uint64(0)

	err := generics.CreateSimplePipeline(him.Client, "select-optimal-report").
		WithSystemMessage(`
You are an Advanced Data Analysis AI, with a primary focus on identifying and comparing the quantity and relevance of entities or data points within reports based on a specific goal.

Primary Objective:
{{goal}}

You are presented with two reports for comparison:
- Report A:
{{repA}}

- Report B:
{{repB}}

In your evaluation, prioritize the identification of entities or data points relevant to the primary objective. Assess which report contains a greater quantity of relevant entities or data, considering any additional information provided only if it enhances the understanding or relevance of these entities.

Ignore the self-valuations and self-reflection in source reports, consider only collected facts and data. 
Recommend which report (A, B, or both) best aligns with the primary objective and provides better evidence based on the quantity and relevance of the entities or data points.`).
		WithVar("goal", goal).
		WithVar("repA", a).
		WithVar("repB", b).
		WithResponseField("evaluation", "detailed analysis focusing on the quantity and relevance of entities or data points").
		WithResponseField("recommendation", "<A|B|both|none>"). // "Based on your analysis, which report(s) would you recommend?"
		//WithResponseField("fallback", "If you cannot make a definitive recommendation, describe any additional information or clarification that would be helpful.").
		WithResultsProcessor(func(_ map[string]interface{}, choice string) error {
			atomic.AddUint64(&resultsAttempted, 1)
			choice = strings.ReplaceAll(choice, "\",\n}", "\"\n}")

			// Define regular expressions for matching report types
			reportARegex := regexp.MustCompile(`"recommendation": "(A|Report A|ReportA|<A>|Report A with the addendum .*)`)
			reportBRegex := regexp.MustCompile(`"recommendation": "(B|Report B|ReportB|<B>)`)
			reportBothRegex := regexp.MustCompile(`"recommendation": "(A and B|A,B|<A,B>|<A, B>|<BOTH A and B>|both)"`)

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
					return err
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
			return nil
		}).Run(os_client.REP_IO)

	him.Printf("Got enough results (%d/%d) [%d vs %d] (err=%v)\n",
		resultsProcessed,
		resultsAttempted,
		votesA, votesB,
		err)

	return votesA > votesB, err
}

func (him *HIM) generateUpdatedReport(goal, a, b string, instructions []string) []string {
	/*updatedReportQuery := tools.NewChatPrompt().
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
				tools.CodeBlock(b))).DefString()*/
	tactics := make([]string, 0)
	if instructions != nil && len(instructions) > 0 {
		tactics = append(tactics, instructions...)
	}
	tactics = append(tactics, `1. **Identify and Extract Relevant Facts**: Carefully review both drafts to extract all relevant facts that align with the primary goal. Ignore any secondary discussions or ancillary comments not directly related to the objective.

2. **Eliminate Redundancies**: Where the two drafts contain overlapping information, consolidate the details to avoid repetition while ensuring no critical fact is omitted.

3. **Organize for Clarity and Coherence**: Re-arrange the extracted information to create a logically structured, easy-to-read document. The structure should facilitate understanding and highlight the most pertinent facts supporting the primary goal.

4. **Maintain Factual Accuracy**: Ensure that the merged report remains true to the facts presented in the original drafts. Any discrepancies or contradictions between the drafts should be noted and addressed appropriately.

Your final output should be a merged report that is comprehensive, factually accurate, and directly supports the primary goal. It should be free of irrelevant commentary and organized in a manner that enhances readability and comprehension.
`)
	requests := make([]cmds.GetCompletionRequest, 0, len(tactics))
	for _, tactic := range shuffle(tactics) {
		updatedReportQuery := tools.NewChatPrompt().
			AddSystem(fmt.Sprintf(`You are an Advanced Report Merging AI, specializing in synthesizing information to support a specific primary goal. Your task is to analyze, compare, and integrate content from two provided drafts into a single, coherent document.

Primary Goal:
%s

Instructions:
%s

Drafts for Comparison and Merging:
- Report A:
%s

- Report B:
%s

Do not reference drafts in the text of combined report. Do not process placeholders and ignore placeholders' content.
Make sure to consolidate all lists first.
Please proceed with the merging process according to these guidelines.`,
				tools.CodeBlock(goal),
				tactic,
				tools.CodeBlock(a),
				tools.CodeBlock(b))).DefString()

		requests = append(requests, cmds.GetCompletionRequest{
			RawPrompt:   updatedReportQuery,
			MinResults:  1,
			Temperature: 0.1,
		})
	}

	minResults := 0
	cycle := 0
retryUpdatedReport:
	minResults++
	cycle++
	for i, _ := range requests {
		requests[i].MinResults = minResults
	}

	updatedReportResponse, err := him.Client.RunRequest(&cmds.ClientRequest{
		ProcessName:           "him-merger",
		GetCompletionRequests: requests,
	}, 600*time.Second, os_client.REP_Default)
	if err != nil {
		time.Sleep(100 * time.Millisecond)
		him.Printf("Error getting response for updated report, going to retry")
		goto retryUpdatedReport
	}

	originalChoices := tools.FlattenChoices(updatedReportResponse.GetCompletionResponse)
	if cycle == 1 && len(originalChoices) >= minResults {
		minResults = len(originalChoices) + 1
		goto retryUpdatedReport
	}

	// filter choices
	choices := make([]string, 0)
	wg := sync.WaitGroup{}
	lock := sync.RWMutex{}
	for _, rep := range originalChoices {
		wg.Add(1)
		go func(rep string) {
			if checkIfReportHasPlaceholdersOrFillers(him.Client, rep, zerolog.Logger{}) {
				return
			}

			if checkIfReportHasAnyDataInIt(him.Client, rep, zerolog.Logger{}) {
				lock.Lock()
				choices = append(choices, rep)
				lock.Unlock()
			}
			wg.Done()
		}(rep)
	}

	wg.Wait()

	for _, updatedReport := range shuffle(choices) {
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
			allOptionsLock := sync.RWMutex{}
			wg := sync.WaitGroup{}
			for _, reportA := range reportsList {
				reportAId := engines.GenerateMessageId(reportA)
				for _, reportB := range reportsList {
					reportBId := engines.GenerateMessageId(reportB)
					if reportAId == reportBId {
						continue
					}
					wg.Add(1)
					go func(goal, reportA, reportB, reportAId, reportBId string) {
						defer wg.Done()
						isBetter, err := him.isReportABetter(goal, reportA, reportB)
						if err != nil {
							return
						}
						if isBetter {
							allOptionsLock.Lock()
							ratings[reportAId]++
							allOptions = append(allOptions, reportA)
							allOptionsLock.Unlock()
						}
					}(goal, reportA, reportB, reportAId, reportBId)
				}
			}

			wg.Wait()

			// remove all duplicates
			results := tools.DropDuplicates(allOptions)
			if len(results) < 2 {
				return results
			}
		}
	}

	return []string{a, b}
}
