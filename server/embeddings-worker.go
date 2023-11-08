package server

import (
	"github.com/d0rc/agent-os/settings"
	"github.com/d0rc/agent-os/vectors"
)

type VectorDBType string

const (
	VDB_QDRANT VectorDBType = "qdrant"
)

func (ctx *Context) backgroundEmbeddingsWorker() {
	// let's see what we have in our vector DBs configs

	for _, vectorDB := range ctx.Config.VectorDBs {
		if vectorDB.Type == string(VDB_QDRANT) {
			err := ctx.startQdrant(&vectorDB)
			if err != nil {
				ctx.Log.Error().
					Err(err).
					Msgf("error starting qdrant server: %v", err)
			}
		}
	}
}

func (ctx *Context) startQdrant(vectorDB *settings.VectorDBConfigurationSection) error {
	vectorDb, err := vectors.NewQdrantClient(vectorDB)
	if err != nil {
		return err
	}

	ctx.VectorDBs = append(ctx.VectorDBs, vectorDb)
	return nil
}
