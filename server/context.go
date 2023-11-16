package server

import (
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/vectors"
	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog"
	"os"
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

type Settings struct {
	TopInterval time.Duration
	TermUI      bool
	LogChan     chan []byte
}

func NewContext(configPath string, lg zerolog.Logger, srvSettings *Settings) (*Context, error) {
	config, err := settings.ProcessConfigurationFile(configPath)
	if err != nil {
		return nil, err
	}

	db, err := storage.NewStorage(lg)
	if err != nil {
		fmt.Printf("error creating storage: %v\n", aurora.BrightRed(err))
		fmt.Printf("confgiration file used: %s\n", configPath)
		os.Exit(1)
	}

	computeRouter := borrow_engine.NewInferenceEngine(borrow_engine.ComputeFunction{
		borrow_engine.JT_Completion: func(n *borrow_engine.InferenceNode, jobs []*borrow_engine.ComputeJob) ([]*borrow_engine.ComputeJob, error) {
			lg.Warn().Msg("completion job received")
			if len(jobs) == 0 {
				return nil, nil
			}
			tasks := make([]*engines.JobQueueTask, len(jobs))
			resChan := make([]chan *engines.Message, len(jobs))
			for idx, job := range jobs {
				resChan[idx] = make(chan *engines.Message, 1)
				tasks[idx] = &engines.JobQueueTask{
					Req: job.GenerationSettings,
					Res: resChan[idx],
				}
			}

			_, err := engines.RunCompletionRequest(n.RemoteEngine, tasks)
			if err != nil {
				lg.Error().Err(err).Msg("error running completion request")
				return nil, err
			}

			for idx, job := range jobs {
				failureTimeout := time.NewTimer(120 * time.Second)
				select {
				case <-failureTimeout.C:
					lg.Error().Msg("completion request timed out")
					return nil, err
				case tmpResult := <-resChan[idx]:
					job.ComputeResult.CompletionChannel <- tmpResult
				}
			}
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
				// TODO: remove timeout...!
				// it was needed to prevent stalled compute
				// but all bugs should be fixed now
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
	}, &borrow_engine.InferenceEngineSettings{
		TopInterval: srvSettings.TopInterval,
		TermUI:      srvSettings.TermUI,
		LogChan:     srvSettings.LogChan,
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

func (ctx *Context) LaunchWorker(name string, worker func(ctx *Context, name string)) {
	go worker(ctx, name)
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
