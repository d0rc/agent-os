package agency

import (
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"sync/atomic"
)

// jobsChannelManager is responsible for getting jobs from agentState.jobsChannel
// and executing these not more than MaxJobsPerAgent at the same time
func (agentState *GeneralAgentInfo) jobsChannelManager() {
	maxJobThreads := make(chan struct{}, MaxJobsPerAgent)
	for {
		select {
		case <-agentState.quitChannelJobs:
			return
		case job := <-agentState.jobsChannel:
			atomic.AddUint64(&agentState.jobsReceived, 1)
			go func(job *cmds.ClientRequest) {
				maxJobThreads <- struct{}{}
				defer func() {
					<-maxJobThreads
					atomic.AddUint64(&agentState.jobsFinished, 1)
				}()
				resp, err := agentState.Server.RunRequest(job, JobsManagerInferenceTimeout, JobsManagerExecutionPool)
				if err != nil {
					fmt.Printf("error running request: %v\n", err)
					go func(job *cmds.ClientRequest) {
						agentState.jobsChannel <- job
					}(job)
				} else {
					agentState.resultsChannel <- resp
				}
			}(job)
		}
	}
}
