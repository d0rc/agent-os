package agency

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	message_store "github.com/d0rc/agent-os/message-store"
	"github.com/d0rc/agent-os/tools"
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
