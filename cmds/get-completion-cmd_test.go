package cmds

import (
	zlog "github.com/rs/zerolog/log"
	"testing"
)

func TestGetCompletionsCmd(t *testing.T) {
	lg := zlog.Logger
	storage, err := NewStorage(lg)
	if err != nil {
		lg.Fatal().Err(err).Msg("init storage failed")
	}

	resp, err := ProcessGetCompletions([]GetCompletionRequest{
		{
			Model: "TheBloke/zephyr-7B-beta-AWQ",
			RawPrompt: `### Instruction
You are Default Mode Agent of AI Bot. 

### Assistant:`,
			Temperature: 0.8,
			StopTokens:  []string{"###"},
			MinResults:  10,
			MaxResults:  1,
		},
	}, storage)

	lg.Info().Err(err).Interface("resp", resp).Msg("get completions")
}
