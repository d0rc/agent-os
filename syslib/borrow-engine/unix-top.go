package borrow_engine

import (
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	osProcess "github.com/shirou/gopsutil/process"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func (ie *InferenceEngine) PrintTop(jobsBuffer map[JobPriority][]*ComputeJob, lock *sync.RWMutex) {
	if ie.settings.TermUI == false {
		// let's create a string builder
		topInfo := ie.buildTopString(jobsBuffer, lock, false)

		fmt.Printf("%s", topInfo.topString)
	}
}

type topDataInfo struct {
	topString      string
	computeEngines [][]string
	topLines       string
	processesLines [][]string
}

func (ie *InferenceEngine) buildTopString(jobsBuffer map[JobPriority][]*ComputeJob, lock *sync.RWMutex, termUi bool) *topDataInfo {
	result := &topDataInfo{
		computeEngines: make([][]string, 0),
	}
	stringBuilder := &strings.Builder{}
	// let's write to it
	// clear screen
	fmt.Fprintf(stringBuilder, "\033[2J")
	topLines := fmt.Sprintf("Total jobs: %s, Total requests: %d, Total time consumed: %s, Total time idle: %s\n",
		makeBrightCyan(termUi, humanize.SIWithDigits(float64(ie.TotalJobsProcessed), 2, "j")),
		ie.TotalRequestsProcessed,
		ie.TotalTimeConsumed,
		ie.TotalTimeIdle)
	topLines = topLines + fmt.Sprintf("Total jobs in buffer: %d(+%d), Total time in scheduler: %s, Uptime: %s\n",
		countMapValueLens(jobsBuffer, lock),
		len(ie.IncomingJobs),
		ie.TotalTimeScheduling,
		getUptime())
	fmt.Fprintf(stringBuilder, topLines)
	result.topLines = topLines
	tw := tablewriter.NewWriter(stringBuilder)

	computeEnginesHeaders := []string{"Endpoint", "Compute State", "Max (reqs/batch)", "Reqs/Jobs", "TimeConsumed", "TimeIdle", "T.Waisted", "Failed(R/J)"}
	tw.SetHeader(computeEnginesHeaders)
	result.computeEngines = append(result.computeEngines, computeEnginesHeaders)

	for _, node := range ie.Nodes {
		computeEnginesLine := []string{
			shoLastNRunes(node.EndpointUrl, 35),
			fmt.Sprintf("%v", getNodeState(termUi, int(atomic.LoadInt32(&node.RequestsRunning)))),
			fmt.Sprintf("%d/%d", node.MaxRequests, node.MaxBatchSize),
			fmt.Sprintf("%d/%d", node.TotalRequestsProcessed, node.TotalJobsProcessed),
			fmt.Sprintf("%s", node.TotalTimeConsumed),
			fmt.Sprintf("%s", node.TotalTimeIdle),
			fmt.Sprintf("%s", node.TotalTimeWaisted),
			fmt.Sprintf("%d/%d", node.TotalRequestsFailed, node.TotalJobsFailed),
		}
		tw.Append(computeEnginesLine)
		result.computeEngines = append(result.computeEngines, computeEnginesLine)
	}
	tw.Render()

	tw = tablewriter.NewWriter(stringBuilder)
	processesHeadersLines := make([][]string, 0)
	processesHeaders := []string{"Process", "TotalRequestsProcessed", "TotalJobsProcessed", "TotalTimeConsumed", "AvgWait"}
	tw.SetHeader(processesHeaders)
	processesHeadersLines = append(processesHeadersLines, processesHeaders)
	lock.RLock()

	type ProcessInfo struct {
		TotalRequests uint64
		TotalJobs     uint64
		Name          string
	}
	processInfo := make([]ProcessInfo, 0, len(ie.ProcessesTotalRequests))
	for process, tr := range ie.ProcessesTotalRequests {
		tj, exists := ie.ProcessesTotalJobs[process]
		if !exists {
			tj = 0
		}
		processInfo = append(processInfo, ProcessInfo{
			TotalRequests: tr,
			TotalJobs:     tj,
			Name:          process,
		})
	}
	// sort processInfo, make process with most jobs first
	// use library sort
	sort.Slice(processInfo, func(i, j int) bool {
		return processInfo[i].TotalRequests > processInfo[j].TotalRequests
	})

	for idx, processData := range processInfo {
		processesHeadersLine := []string{
			processData.Name,
			fmt.Sprintf("%d", processData.TotalRequests),
			fmt.Sprintf("%d", ie.ProcessesTotalJobs[processData.Name]),
			fmt.Sprintf("%s", ie.ProcessesTotalTimeConsumed[processData.Name]),
			fmt.Sprintf("%s", fmt.Sprintf("%4.4f", float64(ie.ProcessesTotalTimeWaiting[processData.Name]/time.Millisecond)/float64(ie.ProcessesTotalJobs[processData.Name]))),
		}
		if idx < 7 {
			tw.Append(processesHeadersLine)
			processesHeadersLines = append(processesHeadersLines, processesHeadersLine)
		}
	}
	lock.RUnlock()
	tw.Render()

	result.topString = stringBuilder.String()
	result.processesLines = processesHeadersLines
	return result
}

func makeBrightCyan(ui bool, digits string) string {
	if !ui {
		return aurora.BrightCyan(digits).String()
	}

	return fmt.Sprintf("[%s](fg:cyan,mod:bold)", digits)
}

func shoLastNRunes(url string, i int) string {
	// show last i runes of url, if url is shorter than i, show whole url
	// prepend with ... if url is longer than i
	if len(url) <= i {
		return url
	}

	return fmt.Sprintf("...%s", url[len(url)-i:])
}

func getNodeState(ui bool, running int) interface{} {
	if running == 0 {
		return makeBrightWhite(ui, "idle")
	}

	return fmt.Sprintf("%s - %s",
		makeBrightGreen(ui, "busy"),
		makeBrightCyan(ui, fmt.Sprintf("%d", running)))
}

func makeBrightGreen(ui bool, s string) string {
	if !ui {
		return aurora.BrightGreen(s).String()
	}

	return fmt.Sprintf("[%s](fg:green,mod:bold)", s)
}

func makeBrightWhite(ui bool, s string) string {
	if !ui {
		return aurora.BrightWhite(s).String()
	}

	return fmt.Sprintf("[%s](fg:white,mod:bold)", s)
}

func getUptime() time.Duration {
	// Get the current process
	pid := int32(os.Getpid())
	p, err := osProcess.NewProcess(pid)
	if err != nil {
		return 0
	}

	// Get the creation time of the process
	createTime, err := p.CreateTime()
	if err != nil {
		return 0
	}

	// Calculate the uptime of the process
	uptime := time.Since(time.Unix(int64(createTime/1000), 0))

	return uptime
}
