package agency

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/tools"
	"github.com/rs/zerolog/log"
	"runtime"
	"time"
)

func (agentState *GeneralAgentInfo) ioRequestsProcessing() {
	maxIoRequestsChan := make(chan struct{}, runtime.NumCPU()-2)
	for {
		select {
		case <-agentState.quitChannelProcessing:
			return
		case message := <-agentState.resultsProcessingChannel:
			go func(message *engines.Message) {
				maxIoRequestsChan <- struct{}{}
				defer func() {
					<-maxIoRequestsChan
				}()
				ioRequests := agentState.TranslateToServerCallsAndRecordHistory([]*engines.Message{message})
				// run all at once
				if len(ioRequests) == 0 {
					// we have to retry generating agent response...!
					return
				}
				ioResponses, err := agentState.Server.RunRequests(ioRequests, 600*time.Second)
				if err != nil {
					fmt.Printf("error running IO request: %v\n", err)
					// we have to retry running request, it's easy, just send message ourselves again
					return
				}

				// fmt.Printf("Got responses: %v\n", res)
				// we've got responses, if we have observations let's put them into the history
				for idx, commandResponse := range ioResponses {
					if commandResponse == nil {
						fmt.Printf("got nothing in server response at index %d\n", idx)
						continue
					}
					for _, observation := range generateObservationFromServerResults(ioRequests[idx], commandResponse, 1024, agentState) {
						messageId := engines.GenerateMessageId(observation)
						//fmt.Printf("got observation: %v\n", observation)
						correlationId := commandResponse.CorrelationId
						agentState.historyAppenderChannel <- &engines.Message{
							ID:      &messageId,
							ReplyTo: map[string]struct{}{correlationId: {}}, // it should be equal to message.ID TODO: check
							Role:    engines.ChatRoleUser,
							Content: observation,
						}

						/*
							_ = os.MkdirAll("observations", os.ModePerm)
							_ = os.WriteFile(fmt.Sprintf("observations/%s-%s", correlationId, messageId),
								[]byte(observation), os.ModePerm)*/
					}
				}
			}(message)
		}
	}
}

func generateObservationFromServerResults(request *cmds.ClientRequest, response *cmds.ServerResponse, maxLength int, agentState *GeneralAgentInfo) []string {
	observations := make([]string, 0)
	observation := ""
	if request.SpecialCaseResponse != "" {
		observations = append(observations, request.SpecialCaseResponse)
		return observations
	}

	if response == nil {
		log.Error().Msgf("Got a nil response from the server")
		return []string{"server returned nothing..!"}
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
				observation += fmt.Sprintf("```\n%s\n```\n", pageResponse.Markdown)
				if len(observation) < maxLength {
					observations = append(observations, observation)
					observation = ""
				} else {
					// ok, we've got the observation, lets reduce over it
					finalResult := make(map[string]interface{})
					tools.DocumentReduce(observation, fmt.Sprintf(`Your goal is to collect facts which help to answer the following question:
%s
Respond in the following JSON format::
{
   "general-information": "write general information here",
   "entities": [],
   "facts": []
}
`, pageResponse.OriginalQuestion), agentState.Server, func(s string) (string, error) {
						ps := ""
						err := tools.ParseJSON(s, func(x string) error {
							ps = x
							return json.Unmarshal([]byte(s), &finalResult)
						})

						return ps, err
					}, "")

					serializedResult, err := json.MarshalIndent(finalResult, "", " ")
					if err == nil {
						// observation = string(serializedResult)
						observations = append(observations, string(serializedResult))
						observation = ""
					}
				}
			}
		}
	}

	if observation != "" {
		observations = append(observations, observation)
	}

	return observations
}
