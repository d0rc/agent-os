package agency

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/tools"
	"github.com/logrusorgru/aurora"
)

func TranslateToServerCalls(depth int, agentState *GeneralAgentInfo, results []*engines.Message) []*cmds.ClientRequest {
	clientRequests := make([]*cmds.ClientRequest, 0)
	for _, res := range results {
		parsedResults, _ := agentState.ParseResponse(res.Content)
		//fmt.Printf("[%d] %s\n", currentDepth, aurora.BrightGreen(res.Content))
		for _, parsedResult := range parsedResults {
			if parsedResult.HasAnyTags("thoughts") {
				fmt.Printf("[%d] thoughts: %s\n", depth, aurora.BrightWhite(parsedResult.Value))
				tools.RunLocalTTS("Current thoughts: " + parsedResult.Value.(string))
			}
			if parsedResult.HasAnyTags("command") {
				v := parsedResult.Value
				argsJson, _ := json.Marshal(v.(map[string]interface{})["args"])
				fmt.Printf("[%d] command: %s, args: %v\n", depth,
					aurora.BrightYellow(v.(map[string]interface{})["name"]),
					aurora.BrightWhite(string(argsJson)))
				commandName, okCommandName := v.(map[string]interface{})["name"].(string)
				argsData, okArgsData := v.(map[string]interface{})["args"].(map[string]interface{})
				if okCommandName && okArgsData {
					clientRequests = append(clientRequests,
						getServerCommand(
							*res.ID,
							commandName,
							argsData))
				}
			}
		}
	}

	return clientRequests
}

var notes = make(map[string]string)

func getServerCommand(resultId string, commandName string, args map[string]interface{}) *cmds.ClientRequest {
	clientRequest := &cmds.ClientRequest{
		GoogleSearchRequests: make([]cmds.GoogleSearchRequest, 0),
		CorrelationId:        resultId,
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
	case "write-note":
		section := args["section"]
		text := args["text"]
		if section == nil || text == nil {
			clientRequest.SpecialCaseResponse = "No section or text specified."
			break
		}
		switch text.(type) {
		case string:
			notes[section.(string)] = text.(string)
		case []interface{}:
			for _, t := range text.([]interface{}) {
				notes[section.(string)] += t.(string)
			}
		}
		clientRequest.SpecialCaseResponse = "Ok, note saved."
	case "read-note":
		section := args["section"]
		if section == nil {
			clientRequest.SpecialCaseResponse = "No section specified."
		} else {
			text, found := notes[section.(string)]
			if found {
				clientRequest.SpecialCaseResponse = text
			} else {
				clientRequest.SpecialCaseResponse = "No note found."
			}
		}
	case "list-notes":
		clientRequest.SpecialCaseResponse = "Notes:\n"
		for section, text := range notes {
			clientRequest.SpecialCaseResponse += fmt.Sprintf("%s: %s\n", section, text)
		}
	case "speak":
		question := args["text"]
		switch question.(type) {
		case string:
			// fmt.Printf("[%d] prey: %s\n", depth, aurora.BrightWhite(parsedResult.Value))
			tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + question.(string))
			tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + question.(string))
			tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + question.(string))
			tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + question.(string))
			tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + question.(string))
		}
	}
	clientRequest.CorrelationId = resultId

	return clientRequest
}
