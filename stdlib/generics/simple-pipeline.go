package generics

import (
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/flosch/pongo2/v6"
	"github.com/tidwall/gjson"
	"strings"
	"time"
)

/*
   	yesCounter := uint64(0)

   	CreateSimplePipeline().
   		WithSystemMessage(`You are Report Comparing AI. You have to pick the best report for the primary goal.
   Primary goal:
   {{goal}}

   Your task is to compare following two reports:
   Report A:
   {{reportA}}

   Report B:
   {{reportB}}

   Please help to choose a report for further processing.
   Are these reports the same?`).
   		WithVar("goal", goal).
   		WithVar("reportA", codeBlock(a)).
   		WithVar("reportB", codeBlock(b)).
   		WithTemperature(0.1).
   		WithAssistantResponsePrefixOnStepNo(1, "```json\n").
   		AddResponseFields("thoughts", "thoughts text, discussing which report is more comprehensive and better aligns with the primary goal").
   		AddResponseFields("reports-are-equal", "<yes|no>").
   		WithMinParsableResults(2).
   		WithResultsProcessor("reports-are-equal", func(response string) error {
   			if strings.ToLower(response) == "yes" {
   				atomic.AddUint64(&yesCounter, 1)
   			}

   			return nil
   		})

   	return yesCounter > 0, nil
*/

type SimplePipeline struct {
	SystemMessage           string
	Vars                    map[string]interface{}
	Temperature             float32
	AssistantResponsePrefix map[int]string
	ResponseFields          []tools.MapKV
	MinParsableResults      int
	ResultsProcessor        map[string]func(string) error
	Client                  *os_client.AgentOSClient
	FullResultProcessor     func(string) error
}

func CreateSimplePipeline(client *os_client.AgentOSClient) *SimplePipeline {
	return &SimplePipeline{
		Vars:                    make(map[string]interface{}),
		AssistantResponsePrefix: make(map[int]string),
		ResponseFields:          make([]tools.MapKV, 0),
		MinParsableResults:      2,
		Temperature:             0.1,
		ResultsProcessor:        make(map[string]func(string) error),
		Client:                  client,
	}
}

func (p *SimplePipeline) WithSystemMessage(systemMessage string) *SimplePipeline {
	p.SystemMessage = systemMessage
	return p
}

func (p *SimplePipeline) WithVar(name string, val interface{}) *SimplePipeline {
	p.Vars[name] = val
	return p
}

func (p *SimplePipeline) WithTemperature(temperature float32) *SimplePipeline {
	p.Temperature = temperature
	return p
}

func (p *SimplePipeline) WithAssistantResponsePrefixOnStepNo(stepNo int, prefix string) *SimplePipeline {
	p.AssistantResponsePrefix[stepNo] = prefix
	return p
}

func (p *SimplePipeline) WithResponseField(key string, value string) *SimplePipeline {
	p.ResponseFields = append(p.ResponseFields, tools.MapKV{Key: key, Value: value})
	return p
}

func (p *SimplePipeline) WithMinParsableResults(minParsableResults int) *SimplePipeline {
	p.MinParsableResults = minParsableResults
	return p
}

func (p *SimplePipeline) WithResultsProcessor(key string, processor func(string) error) *SimplePipeline {
	p.ResultsProcessor[key] = processor
	return p
}

func (p *SimplePipeline) WithFullResultProcessor(processor func(string) error) *SimplePipeline {
	p.FullResultProcessor = processor
	return p
}

func (p *SimplePipeline) Run(executionPool os_client.RequestExecutionPool) error {
	tpl, err := pongo2.FromString(p.SystemMessage)
	if err != nil {
		return err
	}

	jsonBuffer := &strings.Builder{}
	tools.RenderJsonString(p.ResponseFields, jsonBuffer, 0)

	systemMessage, err := tpl.Execute(p.Vars)
	if err != nil {
		return err
	}

	systemMessage = systemMessage + fmt.Sprintf("\nRespond in the following JSON format:\n```json%s```\n",
		jsonBuffer.String())

	minResults := p.MinParsableResults
retry:
	response, err := p.Client.RunRequest(&cmds.ClientRequest{
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:   tools.NewChatPrompt().AddSystem(systemMessage).String(tools.PSAlpaca),
				Temperature: p.Temperature,
				MinResults:  minResults,
			},
		},
	}, 120*time.Second, executionPool)
	if err != nil {
		time.Sleep(100 * time.Millisecond)
		goto retry
	}

	choices := tools.FlattenChoices(response.GetCompletionResponse)
	if len(choices) > minResults {
		minResults = len(choices) + 1
	}

	okResults := 0
	for _, choice := range choices {
		var parsedValue string
		if err := tools.ParseJSON(choice, func(s string) error {
			if p.FullResultProcessor != nil {
				return p.FullResultProcessor(s)
			} else {
				for _, req := range p.ResponseFields {
					parsedValue = gjson.Get(s, req.Key).String()
					if parsedValue == "" {
						return fmt.Errorf("no value parsed")
					}

					if p.ResultsProcessor[req.Key] != nil {
						if err := p.ResultsProcessor[req.Key](parsedValue); err != nil {
							return err
						}
					}
				}
			}

			return nil
		}); err == nil {
			okResults++
		}
	}

	if okResults < p.MinParsableResults {
		minResults++
		goto retry
	}

	return nil
}
