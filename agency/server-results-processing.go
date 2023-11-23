package agency

import (
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"time"
)

func (agentState *GeneralAgentInfo) ioRequestsProcessing() {
	for {
		select {
		case <-agentState.quitChannelProcessing:
			return
		case message := <-agentState.resultsProcessingChannel:
			go func(message *engines.Message) {
				ioRequests := agentState.TranslateToServerCalls([]*engines.Message{message})
				// run all at once
				ioResponses, err := agentState.Server.RunRequests(ioRequests, 600*time.Second)
				if err != nil {
					fmt.Printf("error running IO request: %v\n", err)
					return
				}

				// fmt.Printf("Got responses: %v\n", res)
				// we've got responses, if we have observations let's put them into the history
				for idx, commandResponse := range ioResponses {
					for _, observation := range generateObservationFromServerResults(ioRequests[idx], commandResponse, 1024) {
						messageId := GenerateMessageId(observation)
						fmt.Printf("got observation: %v\n", observation)
						agentState.historyAppenderChannel <- &engines.Message{
							ID:      &messageId,
							ReplyTo: &commandResponse.CorrelationId, // it should be equal to message.ID TODO: check
							Role:    engines.ChatRoleUser,
							Content: observation,
						}
					}
				}
			}(message)
		}
	}
}

func generateObservationFromServerResults(request *cmds.ClientRequest, response *cmds.ServerResponse, maxLength int) []string {
	observations := make([]string, 0)
	observation := ""
	if request.SpecialCaseResponse != "" {
		observations = append(observations, request.SpecialCaseResponse)
		return observations
	}

	if response.GoogleSearchResponse != nil && len(response.GoogleSearchResponse) > 0 {
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

	if len(response.GetPageResponse) > 0 {
		for _, pageResponse := range response.GetPageResponse {
			if pageResponse.Markdown != "" {
				observation += fmt.Sprintf("Page content for \"%s\":\n", pageResponse.Url)
				observation += fmt.Sprintf("%s\n\n", pageResponse.Markdown)
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
