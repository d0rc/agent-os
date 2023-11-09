package server

import (
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/vectors"
	"github.com/rs/zerolog"
)

type Context struct {
	Config    *settings.ConfigurationFile
	Storage   *storage.Storage
	Log       zerolog.Logger
	VectorDBs []vectors.VectorDB
}

func NewContext(configPath string, lg zerolog.Logger) (*Context, error) {
	config, err := settings.ProcessConfigurationFile(configPath)
	if err != nil {
		return nil, err
	}

	db, err := storage.NewStorage(lg)

	return &Context{
		Config:  config,
		Log:     lg.With().Str("cfg-file", configPath).Logger(),
		Storage: db,
	}, nil
}

func (ctx *Context) Run() {
	if len(ctx.Config.Compute) > 0 {
		go ctx.autoDetectCompute()
	} else {
		ctx.Log.Warn().Msg("no compute section in config")
	}
	if len(ctx.Config.VectorDBs) > 0 {
		go ctx.backgroundEmbeddingsWorker()
	} else {
		ctx.Log.Warn().Msg("no vectorDBs section in config")
	}
}

func (ctx *Context) LaunchAgent() {

}
