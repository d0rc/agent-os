package main

import (
	_ "embed"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/utils"
)

/*

	this is an example of low-level tool for agent-os, it's purpose is to create a report
	answering the question `who is Jimmy Apples?`

*/

//go:embed agency.yaml
var agencyYaml []byte

const goal = "Figure out, who is Jimmy Apples."

var termUi = false

func main() {
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting who-is-jimmy-apples")

	agencySettings, err := agency.ParseAgency(agencyYaml)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to parse agency")
	}

	lg.Info().Interface("agencySettings", agencySettings).Msg("parsed agency")

	client := os_client.NewAgentOSClient("http://localhost:9000")
	agentState := agency.NewGeneralAgentState(client, "", agencySettings[0])
	agentContext := &agency.InferenceContext{
		InputVariables: map[string]any{"goal": goal},
		History:        make([][]*engines.Message, 0),
	}
	results, err := agency.GeneralAgentPipelineStep(agentState,
		0,
		3,
		100,
		4,
		agentContext)

	if err != nil {
		lg.Error().Err(err).
			Interface("results", results).
			Msg("failed to run inference")
	} else {
		for _, res := range results {
			fmt.Println(res.Content + "\n")
		}
	}
}
