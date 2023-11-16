package agency

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/logrusorgru/aurora"
)

func TranslateToServerCalls(agentState *GeneralAgentInfo, results []*engines.Message) []*cmds.ClientRequest {
	clientRequests := make([]*cmds.ClientRequest, 0)
	for _, res := range results {
		parsedResults, _ := agentState.ParseResponse(res.Content)
		//fmt.Printf("[%d] %s\n", currentDepth, aurora.BrightGreen(res.Content))
		for _, parsedResult := range parsedResults {
			if parsedResult.HasAnyTags("thoughts") {
				fmt.Printf("[%d] thoughts: %s\n", -1, aurora.BrightWhite(parsedResult.Value))
			}
			if parsedResult.HasAnyTags("command") {
				v := parsedResult.Value
				argsJson, _ := json.Marshal(v.(map[string]interface{})["args"])
				fmt.Printf("[%d] command: %s, args: %v\n", -1,
					aurora.BrightYellow(v.(map[string]interface{})["name"]),
					aurora.BrightWhite(string(argsJson)))
				commandName, okCommandName := v.(map[string]interface{})["name"].(string)
				argsData, okArgsData := v.(map[string]interface{})["args"].(map[string]interface{})
				if okCommandName && okArgsData {
					clientRequests = append(clientRequests,
						getServerCommand(
							commandName,
							argsData))
				}
			}
		}
	}

	return clientRequests
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
