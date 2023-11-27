package os_client

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"time"
)

func ProcessGetCompletions(request []cmds.GetCompletionRequest, ctx *AgentOSClient, process string, priority borrow_engine.JobPriority) (response *cmds.ServerResponse, err error) {
	// the purpose of this function is mirror the functionality
	// available in OS core
	resp, err := ctx.RunRequest(&cmds.ClientRequest{
		GetCompletionRequests: request,
		ProcessName:           process,
		Priority:              priority,
	}, 120*time.Second, REP_IO)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
