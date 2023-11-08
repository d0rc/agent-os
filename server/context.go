package server

import (
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/storage"
	"github.com/rs/zerolog"
)

type Context struct {
	Config  *settings.ConfigurationFile
	Storage *storage.Storage
	Log     zerolog.Logger
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

}

func (ctx *Context) LaunchAgent() {

}
