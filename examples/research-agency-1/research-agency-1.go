package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/utils"
	"github.com/logrusorgru/aurora"
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

		clientRequests := make([]*cmds.ClientRequest, 0)
		for _, res := range results {
			parsedResults, _ := agentState.ParseResponse(res.Content)
			//fmt.Printf("[%d] %s\n", currentDepth, aurora.BrightGreen(res.Content))
			for _, parsedResult := range parsedResults {
				if parsedResult.HasAnyTags("thoughts") {
					fmt.Printf("[%d] thoughts: %s\n", currentDepth, aurora.BrightWhite(parsedResult.Value))
				}
				if parsedResult.HasAnyTags("command") {
					v := parsedResult.Value
					argsJson, _ := json.Marshal(v.(map[string]interface{})["args"])
					fmt.Printf("[%d] command: %s, args: %v\n", currentDepth,
						aurora.BrightYellow(v.(map[string]interface{})["name"]),
						aurora.BrightWhite(string(argsJson)))
					clientRequests = append(clientRequests,
						getServerCommand(v.(map[string]interface{})["name"].(string), v.(map[string]interface{})["args"].(map[string]interface{})))
				}
			}
		}

		if len(clientRequests) > 0 {
			fmt.Printf("Sending %d client requests to server: %v\n", len(clientRequests), clientRequests)
		}

		currentDepth++
	}

	fmt.Printf("Done in %v\n", time.Since(ts))
}

func getServerCommand(commandName string, args map[string]interface{}) *cmds.ClientRequest {
	clientRequest := &cmds.ClientRequest{
		GoogleSearchRequests: make([]cmds.GoogleSearchRequest, 0),
	}
	switch commandName {
	case "bing-search":
		keywords := args["keywords"]
		switch keywords.(type) {
		case string:
			clientRequest.GoogleSearchRequests = append(clientRequest.GoogleSearchRequests, cmds.GoogleSearchRequest{
				Keywords: keywords.(string),
			})
		case []interface{}:
			for _, keyword := range keywords.([]interface{}) {
				clientRequest.GoogleSearchRequests = append(clientRequest.GoogleSearchRequests, cmds.GoogleSearchRequest{
					Keywords: keyword.(string),
				})
			}
		}
	}

	return clientRequest
}
