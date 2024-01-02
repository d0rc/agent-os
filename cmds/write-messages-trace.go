package cmds

import (
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/server"
)

func ProcessWriteMessagesTrace(name string, traces []*engines.Message, ctx *server.Context, processName string) (*ServerResponse, error) {
	for idx := range traces {
		res, err := ctx.Storage.Db.Exec("save-messages-trace",
			*traces[idx].ID,
			name,
			traces[idx].Role,
			traces[idx].Content)
		if err != nil {
			ctx.Log.Error().Err(err).Msgf("failed to save message [%s] trace: %v", *traces[idx].ID, err)
		}

		msgId, err := res.LastInsertId()
		if err != nil {
			ctx.Log.Error().Err(err).Msgf("failed to fetch message id [%s] trace: %v", *traces[idx].ID, err)
		}

		for replyTo := range traces[idx].ReplyTo {
			_, err = ctx.Storage.Db.Exec("save-message-link",
				*traces[idx].ID,
				replyTo)
			if err != nil {
				ctx.Log.Error().Err(err).Msgf("[%d] failed to save message [%s] link[->%s]: %v",
					msgId,
					*traces[idx].ID,
					replyTo,
					err)
			}
		}
	}

	return &ServerResponse{}, nil
}
