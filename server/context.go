package server

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/vectors"
	"github.com/rs/zerolog"
	"time"
)

type Context struct {
	Config               *settings.ConfigurationFile
	Storage              *storage.Storage
	Log                  zerolog.Logger
	VectorDBs            []vectors.VectorDB
	ComputeRouter        *borrow_engine.InferenceEngine
	DefaultEmbeddingsDim int
}

func NewContext(configPath string, lg zerolog.Logger) (*Context, error) {
	config, err := settings.ProcessConfigurationFile(configPath)
	if err != nil {
		return nil, err
	}

	db, err := storage.NewStorage(lg)

	computeRouter := borrow_engine.NewInferenceEngine(borrow_engine.ComputeFunction{
		borrow_engine.JT_Completion: func(n *borrow_engine.InferenceNode, jobs []*borrow_engine.ComputeJob) ([]*borrow_engine.ComputeJob, error) {
			lg.Warn().Msg("completion job received")
			return jobs, nil
		},
		borrow_engine.JT_Embeddings: func(n *borrow_engine.InferenceNode, jobs []*borrow_engine.ComputeJob) ([]*borrow_engine.ComputeJob, error) {
			//			lg.Warn().Msg("embedding job received")
			if len(jobs) == 0 {
				return nil, nil
			}
			tasks := make([]*engines.JobQueueTask, len(jobs))
			resChan := make([]chan *vectors.Vector, len(jobs))
			for idx, job := range jobs {
				resChan[idx] = make(chan *vectors.Vector, 1)
				tasks[idx] = &engines.JobQueueTask{
					Req:           job.GenerationSettings,
					ResEmbeddings: resChan[idx],
				}
			}
			_, err := engines.RunEmbeddingsRequest(n.RemoteEngine, tasks)
			if err != nil {
				// lg.Error().Err(err).Msg("error running embeddings request")
				return nil, err
			}
			//			lg.Warn().Msg("embedding request done")

			for idx, job := range jobs {
				failureTimeout := time.NewTimer(120 * time.Second)
				select {
				case <-failureTimeout.C:
					lg.Error().Msg("embedding request timed out")
					return nil, err
				case tmpResult := <-resChan[idx]:
					job.ComputeResult.EmbeddingChannel <- tmpResult
				}
				//job.ComputeResult.EmbeddingChannel <- <-resChan[idx]
			}
			return jobs, nil
		},
	})

	return &Context{
		Config:        config,
		Log:           lg.With().Str("cfg-file", configPath).Logger(),
		Storage:       db,
		ComputeRouter: computeRouter,
	}, nil
}

func (ctx *Context) GetDefaultEmbeddingDims() uint64 {
	for _, node := range ctx.ComputeRouter.Nodes {
		if node.RemoteEngine.EmbeddingsDims != nil {
			return *node.RemoteEngine.EmbeddingsDims
		}
	}

	return 0
}

func (ctx *Context) Start(onStart func(ctx *Context)) {
	if len(ctx.Config.Compute) > 0 {
		go ctx.ComputeRouter.Run()
		detectedComputes := make([]chan *borrow_engine.InferenceNode, 0, len(ctx.Config.Compute))
		for _, node := range ctx.Config.Compute {
			ctx.Log.Info().Str("url", node.Endpoint).Msg("adding compute node")
			detectedComputes = append(detectedComputes, ctx.ComputeRouter.AddNode(&borrow_engine.InferenceNode{
				EndpointUrl:           node.Endpoint,
				EmbeddingsEndpointUrl: node.EmbeddingsEndpoint,
				MaxRequests:           node.MaxRequests,
				MaxBatchSize:          node.MaxBatchSize,
				JobTypes:              translateJobTypes(node.JobTypes),
			}))
		}
		for _, ch := range detectedComputes {
			gotNode := <-ch
			ctx.Log.Info().Str("url", gotNode.EndpointUrl).Msg("compute node autodetected")
		}
	} else {
		ctx.Log.Warn().Msg("no compute section in config")
	}

	onStart(ctx)
}

func (ctx *Context) LaunchWorker(name string, embeddingsWorker func(ctx *Context, name string)) {
	go embeddingsWorker(ctx, name)
}

func translateJobTypes(types []string) []borrow_engine.JobType {
	jobTypes := make([]borrow_engine.JobType, len(types))
	for i, t := range types {
		switch t {
		case "embeddings":
			jobTypes[i] = borrow_engine.JT_Embeddings
		case "completion":
			jobTypes[i] = borrow_engine.JT_Completion
		}
	}
	return jobTypes
}

func (ctx *Context) LaunchAgent() {

}
