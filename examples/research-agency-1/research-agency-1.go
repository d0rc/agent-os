package main

import (
	_ "embed"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/engines"
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

	client := os_client.NewAgentOSClient("http://localhost:9000")
	agentState := agency.NewGeneralAgentState(client, "", agencySettings[0])
	agentContext := &agency.InferenceContext{
		InputVariables: map[string]any{},
		History:        make([][]*engines.Message, 0),
	}

	currentDepth := 0
	for {
		results, err := agency.GeneralAgentPipelineStep(agentState,
			currentDepth, // currentDepth
			4,            // batchSize
			100,          // maxSamplingAttempts
			4,            // minResults
			agentContext)
		if err != nil {
			lg.Error().Err(err).
				Interface("results", results).
				Msg("failed to run inference")
			continue
		}

		clientRequests := agency.TranslateToServerCalls(agentState, results)
		if len(clientRequests) > 0 {
			res, err := client.RunRequests(clientRequests, 120*time.Second)
			if err != nil {
				lg.Error().Err(err).Msg("failed to send request")
			}
			fmt.Printf("Got responses: %v\n", res)
		}

		currentDepth++
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}
