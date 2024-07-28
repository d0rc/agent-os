package os_client

import (
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/syslib/borrow-engine"
	"time"
)

func ProcessGetCompletions(request []cmds.GetCompletionRequest, ctx *AgentOSClient, process string, priority borrow_engine.JobPriority) (response *cmds.ServerResponse, err error) {
	// the purpose of this function is mirror the functionality
	// available in OS core
	resp := ctx.RunRequest(&cmds.ClientRequest{
		GetCompletionRequests: request,
		ProcessName:           process,
		Priority:              priority,
	}, 120*time.Second, REP_IO)

	return resp, nil
}
