package main

import (
	_ "embed"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/utils"
	"time"
)

//go:embed agency.yaml
var agencyYaml []byte

var termUi = false

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting research-agency-1")

	agencySettings, err := agency.ParseAgency(agencyYaml)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to parse agency")
	}

	lg.Info().Interface("agencySettings", agencySettings).Msg("parsed agency")

	client := os_client.NewAgentOSClient("http://167.235.115.231:9000")
	agentState := agency.NewGeneralAgentState(client, "", agencySettings[0])

	err = agentState.GeneralAgentPipelineRun( // currentDepth
		9,       // batchSize
		100_000, // maxSamplingAttempts
		1,       // minResults
	)
	if err != nil {
		lg.Error().Err(err).
			Msg("failed to run inference")
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}
