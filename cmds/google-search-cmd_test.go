package cmds

import (
	zlog "github.com/rs/zerolog/log"
	"testing"
)

func TestProcessGoogleSearches(t *testing.T) {
	storage, err := NewStorage(zlog.Logger)
	if err != nil {
		zlog.Fatal().Err(err).Msg("init storage failed")
	}

	resp, err := ProcessGoogleSearches([]GoogleSearchRequest{
		{
			Keywords:   "best restaurants",
			Lang:       "en",
			Country:    "it",
			Location:   "Milan, Italy",
			MaxAge:     0,
			MaxRetries: 10,
		},
	}, storage)

	zlog.Info().Err(err).Interface("resp", resp).Msg("process google searches")
}
