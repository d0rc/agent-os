package main

import (
	_ "embed"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
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

	currentDepth := 0
	for {
		results, err := agency.GeneralAgentPipelineStep(agentState,
			currentDepth, // currentDepth
			4,            // batchSize
			10000,        // maxSamplingAttempts
			4,            // minResults
		)
		if err != nil {
			lg.Error().Err(err).
				Interface("results", results).
				Msg("failed to run inference")
			continue
		}

		clientRequests := agency.TranslateToServerCalls(currentDepth, agentState, results)
		if len(clientRequests) > 0 {
			res, err := client.RunRequests(clientRequests, 120*time.Second)
			if err != nil {
				lg.Error().Err(err).Msg("failed to send request")
			}
			// fmt.Printf("Got responses: %v\n", res)
			// we've got responses, if we have observations let's put them into the history
			for _, commandResponse := range res {
				for _, observation := range generateObservationFromServerResults(commandResponse, 1024) {
					messageId := agency.GenerateMessageId(observation)
					agentState.History = append(agentState.History, &engines.Message{
						ID:      &messageId,
						ReplyTo: &commandResponse.CorrelationId,
						Role:    engines.ChatRoleUser,
						Content: observation,
					})
				}
			}
		}

		currentDepth += 2
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}

func generateObservationFromServerResults(response *cmds.ServerResponse, maxLength int) []string {
	observations := make([]string, 0)
	observation := ""
	if len(response.GoogleSearchResponse) > 0 {
		for _, searchResponse := range response.GoogleSearchResponse {
			//observation += fmt.Sprintf("Search results for \"%s\":\n", searchResponse.Keywords)
			for _, searchResult := range searchResponse.URLSearchInfos {
				observation += fmt.Sprintf("%s\n%s\n%s\n\n", searchResult.Title, searchResult.URL, searchResult.Snippet)
				if len(observation) > maxLength {
					observations = append(observations, observation)
					observation = ""
				}
			}
		}
	}

	if observation != "" {
		observations = append(observations, observation)
	}

	return observations
}
