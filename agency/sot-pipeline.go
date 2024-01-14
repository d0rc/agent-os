package agency

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	message_store "github.com/d0rc/agent-os/message-store"
	"time"
)

func (agentState *GeneralAgentInfo) SoTPipeline() {
	semanticSpace := message_store.NewSemanticSpace(1)
	agentState.space = semanticSpace
	systemMessage, err := agentState.getSystemMessage()
	if err != nil {
		return
	}

	_ = semanticSpace.AddMessage(nil, systemMessage)
	for {
		requests := semanticSpace.GetComputeRequests(1, 1)
		if len(requests) == 0 {
			time.Sleep(5000 * time.Millisecond)
			continue
		}

		// if got here we have a requests to execute...
		for _, request := range requests {

			agentState.jobsChannel <- &cmds.ClientRequest{
				ProcessName: agentState.SystemName,
				Priority:    borrow_engine.PRIO_User,
				GetCompletionRequests: []cmds.GetCompletionRequest{
					{
						RawPrompt:   chatToRawPrompt(semanticSpace.TrajectoryToMessages(request)),
						MinResults:  1,
						Temperature: 0.9,
					},
				},
				CorrelationId: string(message_store.GenerateTrajectoryID(*request)),
			}
		}
	}
}
