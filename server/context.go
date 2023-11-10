package server

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/vectors"
	"github.com/rs/zerolog"
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
		borrow_engine.JT_Completion: func(n *borrow_engine.InferenceNode, jobs []*borrow_engine.ComputeJob) []*borrow_engine.ComputeJob {
			return jobs
		},
		borrow_engine.JT_Embeddings: func(n *borrow_engine.InferenceNode, jobs []*borrow_engine.ComputeJob) []*borrow_engine.ComputeJob {
			return jobs
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

func (ctx *Context) Start(embeddingsWorker func(ctx *Context)) {
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
	if len(ctx.Config.VectorDBs) > 0 {
		go embeddingsWorker(ctx)
	} else {
		ctx.Log.Warn().Msg("no vectorDBs section in config")
	}
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
