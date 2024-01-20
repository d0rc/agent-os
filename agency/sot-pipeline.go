package agency

import (
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/stdlib/message-store"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/d0rc/agent-os/syslib/borrow-engine"
)

func (agentState *GeneralAgentInfo) SoTPipeline(growthFactor, maxRequests, maxPendingRequests int) {
	semanticSpace := message_store.NewSemanticSpace(growthFactor)
	agentState.space = semanticSpace
	systemMessage, err := agentState.getSystemMessage()
	if err != nil {
		return
	}

	_ = semanticSpace.AddMessage(nil, systemMessage)
	waitCount := 0
	for {
		requests := semanticSpace.GetComputeRequests(maxRequests, maxPendingRequests)
		if len(requests) == 0 {
			if agentState.space.Wait() {
				waitCount++
			}

			continue
		}

		// if got here we have a requests to execute...
		for _, request := range requests {
			agentState.jobsChannel <- &cmds.ClientRequest{
				ProcessName: agentState.SystemName,
				Priority:    borrow_engine.PRIO_User,
				GetCompletionRequests: []cmds.GetCompletionRequest{
					{
						RawPrompt:   tools.NewChatPromptWithMessages(semanticSpace.TrajectoryToMessages(request)).DefString(),
						MinResults:  agentState.space.GetGrowthFactor() * 3,
						Temperature: 0.9,
					},
				},
				CorrelationId: string(message_store.GenerateTrajectoryID(*request)),
			}
		}
	}
}
