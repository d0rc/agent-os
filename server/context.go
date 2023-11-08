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
		Log:     lg,
		Storage: db,
	}, nil
}

func (ctx *Context) Run() {
	if len(ctx.Config.VectorDBs) > 0 {
		go ctx.backgroundEmbeddingsWorker()
	}
}

func (ctx *Context) LaunchAgent() {

}
