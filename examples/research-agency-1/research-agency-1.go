package main

import (
	_ "embed"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/utils"
	"strings"
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

	var spawningCallback func(name, goal string) chan string
	spawningCallback = func(name, goal string) chan string {
		clonedSettings, err := agency.ParseAgency(agencyYaml)
		if err != nil {
			lg.Fatal().Err(err).Msg("failed to parse agency")
		}
		clonedSettings[0].Agent.Name = name
		promptLines := strings.Split(clonedSettings[0].Agent.PromptBased.Prompt, "\n")
		promptLines[0] = fmt.Sprintf("You are an AI Agent, your role is - %s. Your current goal is - %s",
			name, goal)
		clonedSettings[0].Agent.PromptBased.Prompt = strings.Join(promptLines, "\n")

		finalPrompt := strings.Join(promptLines, "\n")
		clonedSettings[0].Agent.PromptBased.Prompt = finalPrompt

		newAgentState := agency.NewGeneralAgentState(client, "", clonedSettings[0])
		finalReportsStream := make(chan string, 10)

		newAgentState.FinalReportChannel = finalReportsStream
		newAgentState.ForkCallback = spawningCallback
		go newAgentState.ToTPipeline()

		return finalReportsStream
	}

	agentState.ForkCallback = spawningCallback
	agentState.ToTPipeline()

	fmt.Printf("Done in %v\n", time.Since(ts))
}
