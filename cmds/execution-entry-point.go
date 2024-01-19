package cmds

import (
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/syslib/borrow-engine"
	"github.com/d0rc/agent-os/syslib/server"
	"github.com/d0rc/agent-os/vectors"
	"github.com/google/uuid"
)

func SendComputeRequest(ctx *server.Context,
	process string,
	jobType borrow_engine.JobType,
	jobPriority borrow_engine.JobPriority,
	req *engines.GenerationSettings) *borrow_engine.ComputeResult {
	computeResult := &borrow_engine.ComputeResult{
		CompletionChannel: make(chan *engines.Message, 1),
		EmbeddingChannel:  make(chan *vectors.Vector, 1),
	}

	// ctx.Log.Info().Msgf("Sending compute request for process %s, job type %s, job priority %s",
	//	process, jobType, jobPriority)
	ctx.ComputeRouter.AddJob(&borrow_engine.ComputeJob{
		JobId:              uuid.New().String(),
		JobType:            jobType,
		Priority:           jobPriority,
		Process:            process,
		GenerationSettings: req,
		ComputeResult:      computeResult,
	})

	// ctx.Log.Info().Msgf("Compute request sent for process %s, job type %s, job priority %s",
	//	process, jobType, jobPriority)

	return computeResult
}
