package cmds

import (
	zlog "github.com/rs/zerolog/log"
	"testing"
)

func TestGetPageCmd(t *testing.T) {
	lg := zlog.Logger
	storage, err := NewStorage(lg)
	if err != nil {
		lg.Fatal().Err(err).Msg("init storage failed")
	}

	resp, err := ProcessPageRequests([]GetPageRequest{
		{
			Url: "https://github.com/fschmid56/efficientat",
		},
	}, storage)

	lg.Info().Err(err).Interface("resp", resp).Msg("get page cmd")
}
