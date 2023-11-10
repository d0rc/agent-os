package borrow_engine

import (
	"fmt"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"os"
	"sort"
	"sync"
	"time"
)

func (ie *InferenceEngine) PrintTop(jobsBuffer map[RequestPriority][]*ComputeJob, lock *sync.RWMutex) {
	topLines := fmt.Sprintf("Total jobs: %s, Total requests: %d, Total time consumed: %s, Total time idle: %s\n",
		aurora.BrightCyan(humanize.SIWithDigits(float64(ie.TotalJobsProcessed), 2, "j")),
		ie.TotalRequestsProcessed,
		ie.TotalTimeConsumed,
		ie.TotalTimeIdle)
	topLines = topLines + fmt.Sprintf("Total jobs in buffer: %d(+%d), Total time in scheduler: %s\n",
		countMapValueLens(jobsBuffer, lock),
		len(ie.IncomingJobs),
		ie.TotalTimeScheduling)
	fmt.Printf(topLines)
	tw := tablewriter.NewWriter(os.Stdout)
	tw.SetHeader([]string{"Endpoint", "Requests", "MaxRequests", "MaxBatchSize", "TotalJobsProcessed", "TotalRequestsProcessed", "TotalTimeConsumed", "TotalTimeIdle"})
	for _, node := range ie.Nodes {
		tw.Append([]string{
			node.EndpointUrl,
			fmt.Sprintf("%d", node.RequestsRunning),
			fmt.Sprintf("%d", node.MaxRequests),
			fmt.Sprintf("%d", node.MaxBatchSize),
			fmt.Sprintf("%d", node.TotalJobsProcessed),
			fmt.Sprintf("%d", node.TotalRequestsProcessed),
			fmt.Sprintf("%s", node.TotalTimeConsumed),
			fmt.Sprintf("%s", node.TotalTimeIdle),
		})
	}
	tw.Render()

	tw = tablewriter.NewWriter(os.Stdout)
	tw.SetHeader([]string{"Process", "TotalJobsProcessed", "TotalTimeConsumed", "AvgWait"})
	lock.RLock()

	type ProcessInfo struct {
		TotalJobs uint64
		Name      string
	}
	processInfo := make([]ProcessInfo, 0, len(ie.ProcessesTotalJobs))
	for process, tj := range ie.ProcessesTotalJobs {
		processInfo = append(processInfo, ProcessInfo{
			TotalJobs: tj,
			Name:      process,
		})
	}
	// sort processInfo, make process with most jobs first
	// use library sort
	sort.Slice(processInfo, func(i, j int) bool {
		return processInfo[i].TotalJobs > processInfo[j].TotalJobs
	})

	for _, processData := range processInfo {
		tw.Append([]string{
			processData.Name,
			fmt.Sprintf("%d", ie.ProcessesTotalJobs[processData.Name]),
			fmt.Sprintf("%s", ie.ProcessesTotalTimeConsumed[processData.Name]),
			fmt.Sprintf("%s", fmt.Sprintf("%4.4f", float64(ie.ProcessesTotalTimeWaiting[processData.Name]/time.Millisecond)/float64(ie.ProcessesTotalJobs[processData.Name]))),
		})
	}
	lock.RUnlock()
	tw.Render()
}
