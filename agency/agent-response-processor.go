package agency

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/stdlib/message-store"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/logrusorgru/aurora"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var valuesMap = map[int]uint64{}
var valuesLock = sync.RWMutex{}
var votingErrorCount = uint64(0)
var commandsSkipped = uint64(0)
var commandsApproved = uint64(0)

var lastPrint = time.Now()
var lastPrintLock = sync.RWMutex{}

func (agentState *GeneralAgentInfo) TranslateToServerCallsAndRecordHistory(results []*engines.Message) []*cmds.ClientRequest {
	clientRequests := make([]*cmds.ClientRequest, 0)
	for _, res := range results {
		parsedResults, parsedString, reconstructedParsedJson, err := agentState.ParseResponse(res.Content)
		if err != nil {
			agentState.space.CancelPendingRequest(message_store.TrajectoryID(keys(res.ReplyTo)[0]))
			continue
		}

		// let's go to cross roads here, to see if we should dive deeper here
		voteRating, err := agentState.VoteForAction(agentState.InputVariables[IV_GOAL].(string), reconstructedParsedJson)
		if err != nil {
			atomic.AddUint64(&votingErrorCount, 1)
			fmt.Printf("Error voting for action: %v\n", err)
			agentState.space.CancelPendingRequest(message_store.TrajectoryID(keys(res.ReplyTo)[0]))
			continue
		}
		if voteRating < MinimalVotingRatingForCommand {
			atomic.AddUint64(&commandsSkipped, 1)
			//fmt.Printf("Skipping message %d of %d with rating: %f\n", resIdx, len(results), voteRating)
			agentState.space.CancelPendingRequest(message_store.TrajectoryID(keys(res.ReplyTo)[0]))
			continue
		}
		atomic.AddUint64(&commandsApproved, 1)

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
		// get trajectoryId to which observations will go now...!
		sourceTrajectoryId := message_store.TrajectoryID(keys(correctedMessage.ReplyTo)[0])
		responseTrajectoryId, err := agentState.space.GetNextTrajectoryID(sourceTrajectoryId,
			message_store.MessageID(msgId))

		reactiveResultSink := func(msgId, content string) {
			reactiveResponseId := engines.GenerateMessageId(content)
			reactiveResponse := &engines.Message{
				ID: &reactiveResponseId,
				//ReplyTo:  map[string]struct{}{*correctedMessage.ID: {}},
				ReplyTo:  map[string]struct{}{string(responseTrajectoryId): {}},
				MetaInfo: res.MetaInfo,
				Role:     engines.ChatRoleUser,
				Content:  content,
			}
			agentState.historyAppenderChannel <- reactiveResponse
		}

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
								string(responseTrajectoryId),
								commandName,
								argsData,
								reactiveResultSink)...)
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
									string(responseTrajectoryId),
									commandName,
									argsData,
									reactiveResultSink)...)
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

	lastPrintLock.RLock()
	timeSince := time.Since(lastPrint)
	lastPrintLock.RUnlock()
	if timeSince > 1*time.Second {
		lastPrintLock.Lock()
		lastPrint = time.Now()
		lastPrintLock.Unlock()
		fmt.Printf("[voter-stats] commands approved: %d, skipped: %d, errors: %d\n",
			aurora.BrightGreen(atomic.LoadUint64(&commandsApproved)),
			aurora.BrightCyan(atomic.LoadUint64(&commandsSkipped)),
			aurora.BrightRed(atomic.LoadUint64(&votingErrorCount)))
	}

	return clientRequests
}

var notesLock = sync.RWMutex{}
var notes = make(map[string][]string)
var listAllNotesSubs = []func(string, string){}
var sectionSubs = make(map[string][]func(string, string))

func (agentState *GeneralAgentInfo) getServerCommand(resultId string,
	commandName string,
	args map[string]interface{},
	reactiveResultSink func(string, string)) []*cmds.ClientRequest {
	clientRequests := make([]*cmds.ClientRequest, 0)
	clientRequests = append(clientRequests, &cmds.ClientRequest{
		GoogleSearchRequests: make([]cmds.GoogleSearchRequest, 0),
		ProcessName:          agentState.SystemName,
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
				keywordString, ok := keyword.(string)
				if ok {
					clientRequests[0].GoogleSearchRequests = append(clientRequests[0].GoogleSearchRequests, cmds.GoogleSearchRequest{
						Keywords: keywordString,
					})
				}
			}
		}
	case "write-note":
		section := args["section"]
		text := args["text"]
		if section == nil || text == nil {
			clientRequests[0].SpecialCaseResponse = "No section or text specified."
			break
		}
		var sectionString string
		switch section.(type) {
		case string:
			sectionString = section.(string)
		}

		if sectionString == "" {
			clientRequests[0].SpecialCaseResponse = "No single section name specified."
			break
		}

		notesLock.Lock()
		if notes[sectionString] == nil {
			notes[sectionString] = make([]string, 0)
			// it's a new section, let's re-list all notes now
			notesList := listAllNotes()
			for _, sub := range listAllNotesSubs {
				sub(resultId, notesList)
			}
		}
		var textToAdd = ""
		switch text.(type) {
		case string:
			textToAdd = text.(string)
		default:
			jsonText, _ := json.Marshal(text)
			textToAdd = string(jsonText)
		}
		notes[sectionString] = append(notes[sectionString], textToAdd)

		// now, let's check if there are subscriptions for this section
		if subs, exists := sectionSubs[sectionString]; exists {
			for _, sub := range subs {
				sub(resultId, textToAdd)
			}
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
			var sectionName string
			var found bool = false
			switch section.(type) {
			case string:
				sectionName = section.(string)
				texts, found = notes[sectionName]
			case []interface{}:
				if len(section.([]interface{})) == 1 {
					sectionName = section.([]interface{})[0].(string)
					texts, found = notes[sectionName]
				}
			}
			notesLock.RUnlock()
			notesLock.Lock()
			if sectionSubs[sectionName] == nil {
				sectionSubs[sectionName] = make([]func(string, string), 0)
			}
			sectionSubs[sectionName] = append(sectionSubs[sectionName], reactiveResultSink)
			notesLock.Unlock()
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
		notesLock.RLock()
		notesList := listAllNotes()
		notesLock.RUnlock()
		clientRequests[0].SpecialCaseResponse = notesList
		notesLock.Lock()
		listAllNotesSubs = append(listAllNotesSubs, reactiveResultSink)
		notesLock.Unlock()
	case "final-report", "periodic-report", "interim-report":
		data := args["text"]
		switch data.(type) {
		case string:
			if agentState.FinalReportChannel != nil {
				agentState.FinalReportChannel <- data.(string)
			} else {
				// fmt.Printf("[%d] prey: %s\n", depth, aurora.BrightWhite(parsedResult.Value))
				tools.RunLocalTTS("WARNING!!!!! I'm speaking!!!! " + data.(string))
				tools.AppendFile("say.log", data.(string))
			}
		}
	case "hire-agent":
		if agentState.ForkCallback != nil {
			roleNameInterface, exists := args["role-name"]
			if exists {
				roleName, roleNameOk := roleNameInterface.(string)
				if roleNameOk {
					taskDescriptionInterface := args["task-description"]
					if taskDescriptionInterface != nil {
						var taskDescription string
						taskDescription, ok := taskDescriptionInterface.(string)
						if ok {
							fmt.Printf("Hiring agent: %s, to execute task: %s\n",
								aurora.BrightWhite(roleName),
								aurora.BrightYellow(taskDescription))
							go func(roleName, taskDescription, resultId string) {

								for msg := range agentState.ForkCallback(args["role-name"].(string), args["task-description"].(string)) {
									// we've got final report from our sub-agent
									fmt.Printf("Got sub-agent's final report: %s\n", msg)
									content := fmt.Sprintf("Final report from %s:\n```\n%s\n```",
										roleName, msg)
									contentMessageId := engines.GenerateMessageId(content)
									tools.AppendFile("final-reports.log", fmt.Sprintf("Final report from %s:\nTask description: %s\nFinal report: %s\n\n\n",
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

func listAllNotes() string {
	notesList := "Notes:\n"
	for section, _ := range notes {
		notesList += fmt.Sprintf("- %s\n", section)
	}
	return notesList
}
