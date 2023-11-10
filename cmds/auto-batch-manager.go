package cmds

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/server"
	"github.com/google/uuid"
)

func SendComputeRequest(ctx *server.Context, req *engines.GenerationSettings) chan *engines.Message {
	outputChannel := make(chan *engines.Message, 1)

	ctx.ComputeRouter.AddJob(&borrow_engine.ComputeJob{
		JobId:              uuid.New().String(),
		JobType:            borrow_engine.JT_Completion,
		Priority:           borrow_engine.PRIO_User,
		Process:            "vllm",
		GenerationSettings: req,
	})

	return outputChannel
}
