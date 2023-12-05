package agency

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/tools"
	"github.com/logrusorgru/aurora"
	"net/url"
	"os"
	"sync"
)

func (agentState *GeneralAgentInfo) TranslateToServerCallsAndRecordHistory(results []*engines.Message) []*cmds.ClientRequest {
	clientRequests := make([]*cmds.ClientRequest, 0)
	for resIdx, res := range results {
		parsedResults, parsedString, err := agentState.ParseResponse(res.Content)
		if err != nil {
			continue
		}

		// let's go to cross roads here, to see if we should dive deeper here
		voteRating, err := agentState.VoteForAction(agentState.Settings.GetAgentInitialGoal(), parsedString)
		if err != nil {
			fmt.Printf("Error voting for action: %v\n", err)
			continue
		}
		if voteRating < MinimalVotingRatingForCommand {
			fmt.Printf("Skipping message %d of %d with rating: %f\n", resIdx, len(results), voteRating)
			continue
		}

		// it's only "parsedString" substring of original model response is interpretable by the system
		msgId := engines.GenerateMessageId(parsedString)
		correctedMessage := &engines.Message{
			ID:       &msgId,
			ReplyTo:  res.ReplyTo,
			MetaInfo: res.MetaInfo,
			Role:     res.Role,
			Content:  parsedString,
		}
		agentState.historyAppenderChannel <- correctedMessage

		//fmt.Printf("[%d] %s\n", currentDepth, aurora.BrightGreen(res.Content))
		for _, parsedResult := range parsedResults {
			if parsedResult.HasAnyTags("thoughts") {
				fmt.Printf("thoughts: %s\n", aurora.BrightWhite(parsedResult.Value))
				// tools.RunLocalTTS("Current thoughts: " + parsedResult.Value.(string))
			}
			if parsedResult.HasAnyTags("command") {
				v := parsedResult.Value
				switch v.(type) {
				case map[string]interface{}:
					argsJson, _ := json.Marshal(v.(map[string]interface{})["args"])
					fmt.Printf("command: %s, args: %v\n",
						aurora.BrightYellow(v.(map[string]interface{})["name"]),
						aurora.BrightWhite(string(argsJson)))
					commandName, okCommandName := v.(map[string]interface{})["name"].(string)
					argsData, okArgsData := v.(map[string]interface{})["args"].(map[string]interface{})
					if okCommandName && okArgsData {
						clientRequests = append(clientRequests,
							agentState.getServerCommand(
								*correctedMessage.ID,
								commandName,
								argsData)...)
					}
				case []map[string]interface{}:
					// ok, it's a list of commands, for a fuck sake...
					cmdList, _ := v.([]map[string]interface{})
					for _, cmd := range cmdList {
						argsJson, _ := json.Marshal(cmd["args"])
						fmt.Printf("command: %s, args: %v\n",
							aurora.BrightYellow(cmd["name"]),
							aurora.BrightWhite(string(argsJson)))
						commandName, okCommandName := cmd["name"].(string)
						argsData, okArgsData := cmd["args"].(map[string]interface{})
						if okCommandName && okArgsData {
							clientRequests = append(clientRequests,
								agentState.getServerCommand(
									*correctedMessage.ID,
									commandName,
									argsData)...)
						}
					}
				default:
					argsJson, _ := json.Marshal(v)
					fmt.Printf("command: %v, args: %v\n",
						aurora.BrightRed(v),
						aurora.BrightWhite(string(argsJson)))
				}
			}
		}
	}

	return clientRequests
}

var notesLock = sync.RWMutex{}
var notes = make(map[string][]string)

func (agentState *GeneralAgentInfo) getServerCommand(resultId string, commandName string, args map[string]interface{}) []*cmds.ClientRequest {
	clientRequests := make([]*cmds.ClientRequest, 0)
	clientRequests = append(clientRequests, &cmds.ClientRequest{
		GoogleSearchRequests: make([]cmds.GoogleSearchRequest, 0),
		CorrelationId:        resultId,
	})
	switch commandName {
	case "bing-search":
		keywords := args["keywords"]
		switch keywords.(type) {
		case string:
			clientRequests[0].GoogleSearchRequests = append(clientRequests[0].GoogleSearchRequests, cmds.GoogleSearchRequest{
				Keywords: keywords.(string),
			})
		case []interface{}:
			for _, keyword := range keywords.([]interface{}) {
				clientRequests[0].GoogleSearchRequests = append(clientRequests[0].GoogleSearchRequests, cmds.GoogleSearchRequest{
					Keywords: keyword.(string),
				})
			}
		}
	case "write-note":
		section := args["section"]
		text := args["text"]
		if section == nil || text == nil {
			clientRequests[0].SpecialCaseResponse = "No section or text specified."
			break
		}
		sectionString := section.(string)

		notesLock.Lock()
		if notes[sectionString] == nil {
			notes[sectionString] = make([]string, 0)
		}
		switch text.(type) {
		case string:
			notes[sectionString] = append(notes[sectionString], text.(string))
		default:
			jsonText, _ := json.Marshal(text)
			notes[sectionString] = append(notes[sectionString], string(jsonText))
		}
		notesLock.Unlock()
		clientRequests[0].SpecialCaseResponse = "Ok, note saved."

		notesLock.RLock()
		allNotesJson, err := json.Marshal(notes)
		notesLock.RUnlock()
		if err == nil {
			_ = os.WriteFile("/tmp/ai-notes.json", allNotesJson, 0644)
		}
	case "read-note":
		section := args["section"]
		if section == nil {
			clientRequests[0].SpecialCaseResponse = "No section specified."
		} else {
			notesLock.RLock()
			var texts []string
			var found bool = false
			switch section.(type) {
			case string:
				texts, found = notes[section.(string)]
			case []interface{}:
				if len(section.([]interface{})) == 1 {
					texts, found = notes[section.([]interface{})[0].(string)]
				}
			}
			notesLock.RUnlock()
			if found {
				// we should return all possible notes here, so...
				if len(texts) == 1 {
					clientRequests[0].SpecialCaseResponse = texts[len(texts)-1]
				} else {
					for idx, text := range texts {
						if idx == 0 {
							clientRequests[0].SpecialCaseResponse = texts[len(texts)-1]
						} else {
							clientRequests = append(clientRequests, &cmds.ClientRequest{
								CorrelationId:       resultId,
								SpecialCaseResponse: text,
							})
						}
					}
				}
			} else {
				clientRequests[0].SpecialCaseResponse = "No note found."
			}
		}
	case "list-notes":
		clientRequests[0].SpecialCaseResponse = "Notes:\n"
		notesLock.RLock()
		for section, text := range notes {
			clientRequests[0].SpecialCaseResponse += fmt.Sprintf("%s: %s\n", section, text)
		}
		notesLock.RUnlock()
	case "final-report":
		data := args["text"]
		switch data.(type) {
		case string:
			if agentState.FinalReportChannel != nil {
				agentState.FinalReportChannel <- data.(string)
			} else {
				// fmt.Printf("[%d] prey: %s\n", depth, aurora.BrightWhite(parsedResult.Value))
				tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + data.(string))
				appendFile("say.log", data.(string))
			}
		}
	case "hire-agent":
		fmt.Printf("Hiring agent: %s\n", args["name"])
		if agentState.ForkCallback != nil {
			roleNameInterface, exists := args["role-name"]
			if exists {
				roleName := roleNameInterface.(string)
				taskDescriptionInterface := args["task-description"]
				if taskDescriptionInterface != nil {
					var taskDescription string
					taskDescription = taskDescriptionInterface.(string)
					go func(roleName, taskDescription, resultId string) {
						for msg := range agentState.ForkCallback(args["role-name"].(string), args["task-description"].(string)) {
							// we've got final report from our sub-agent
							fmt.Printf("Got sub-agent's final report: %s\n", msg)
							content := fmt.Sprintf("Final report from %s:\n```\n%s\n```",
								roleName, msg)
							contentMessageId := engines.GenerateMessageId(content)
							appendFile("final-reports.log", fmt.Sprintf("Final report from %s:\nTask description: %s\nFinal report: %s\n\n\n",
								roleName, taskDescription, msg))
							agentState.historyAppenderChannel <- &engines.Message{
								ID:      &contentMessageId,
								ReplyTo: map[string]struct{}{resultId: {}},
								Role:    engines.ChatRoleUser,
								Content: content,
							}
						}
					}(roleName, taskDescription, resultId)
				}
			}
		}
	case "browse-site":
		var urls []string = make([]string, 0)
		var questions []string = make([]string, 0)
		gotError := false
		if args["url"] != nil {
			switch args["url"].(type) {
			case string:
				urls = append(urls, args["url"].(string))
			case []interface{}:
				for _, u := range args["url"].([]interface{}) {
					switch u.(type) {
					case string:
						urls = append(urls, u.(string))
					}
				}
			}
		} else {
			clientRequests[0].SpecialCaseResponse = "No url specified."
			gotError = true
		}
		if args["question"] != nil {
			switch args["question"].(type) {
			case string:
				questions = append(questions, args["question"].(string))
			case []interface{}:
				for _, q := range args["question"].([]interface{}) {
					switch q.(type) {
					case string:
						questions = append(questions, q.(string))
					}
				}
			}
		} else {
			clientRequests[0].SpecialCaseResponse = "No question specified."
			gotError = true
		}

		for _, subUrl := range urls {
			// check url is not malformed:
			_, err := url.ParseRequestURI(subUrl)
			if err != nil {
				clientRequests[0].SpecialCaseResponse = fmt.Sprintf("Malformed URL: %s", subUrl)
				gotError = true
				break
			}
		}

		if !gotError {
			fmt.Printf("Browsing site: %v - %v, err: %v\n", urls, questions, gotError)
			for _, subUrl := range urls {
				for _, subQuestion := range questions {
					clientRequests[0].GetPageRequests = append(clientRequests[0].GetPageRequests, cmds.GetPageRequest{
						Url:           subUrl,
						Question:      subQuestion,
						ReturnSummary: false,
					})
				}
			}
		} else {

		}
	case "none":
		fmt.Printf("No command found.\n")
	default:
		fmt.Printf("Unknown command: %s\n", commandName)
	}
	clientRequests[0].CorrelationId = resultId

	return clientRequests
}

func appendFile(fname string, text string) {
	// append to log file fname, create it if it doesn't exist
	f, err := os.OpenFile(fname, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("failed opening file: %s\n", err)
		return
	}

	defer f.Close()

	if _, err := f.WriteString(text + "\n"); err != nil {
		fmt.Printf("failed writing to file: %s\n", err)
		return
	}
}
