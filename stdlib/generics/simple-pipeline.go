package generics

import (
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	agent_tools "github.com/d0rc/agent-os/stdlib/agent-tools"
	"github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/flosch/pongo2/v6"
	"github.com/tidwall/gjson"
	"strings"
	"time"
)

type ResultProcessingOutcome int

const (
	RPOFailed ResultProcessingOutcome = iota
	RPOIgnored
	RPOProcessed
)

type ResultsProcessingFunction func(string) (ResultProcessingOutcome, error)

type SimplePipeline struct {
	SystemMessage           string
	Vars                    map[string]interface{}
	Temperature             float32
	AssistantResponsePrefix map[int]string
	ResponseFields          []tools.MapKV
	MinParsableResults      int
	ResultsProcessor        map[string]ResultsProcessingFunction
	Client                  *os_client.AgentOSClient
	FullResultProcessor     ResultsProcessingFunction
	ProcessName             string
	Tools                   []agent_tools.AgentTool
	History                 []*engines.Message
}

func CreateSimplePipeline(client *os_client.AgentOSClient) *SimplePipeline {
	return &SimplePipeline{
		Vars:                    make(map[string]interface{}),
		AssistantResponsePrefix: make(map[int]string),
		ResponseFields:          make([]tools.MapKV, 0),
		MinParsableResults:      2,
		Temperature:             0.1,
		ResultsProcessor:        make(map[string]ResultsProcessingFunction),
		Client:                  client,
	}
}

func (p *SimplePipeline) WithSystemMessage(systemMessage string) *SimplePipeline {
	p.SystemMessage = systemMessage
	return p
}

func (p *SimplePipeline) WithProcessName(name string) *SimplePipeline {
	p.ProcessName = name
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

func (p *SimplePipeline) WithResultsProcessor(key string, processor ResultsProcessingFunction) *SimplePipeline {
	p.ResultsProcessor[key] = processor
	return p
}

func (p *SimplePipeline) WithFullResultProcessor(processor ResultsProcessingFunction) *SimplePipeline {
	p.FullResultProcessor = processor
	return p
}

func (p *SimplePipeline) WithTools(tools ...agent_tools.AgentTool) *SimplePipeline {
	p.Tools = tools
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

	if p.Tools != nil && len(p.Tools) > 0 {
		if !strings.HasSuffix(systemMessage, "\n\n") {
			if !strings.HasSuffix(systemMessage, "\n") {
				systemMessage += "\n\n"
			} else {
				systemMessage += "\n"
			}
		}

		systemMessage = systemMessage + agent_tools.GetContextDescription(p.Tools)
	}

	if len(p.ResponseFields) > 0 {
		systemMessage = systemMessage + fmt.Sprintf("\nRespond in the following JSON format:\n```json%s```\n",
			jsonBuffer.String())
	}

	minResults := p.MinParsableResults
	parsedChoices := make(map[string]struct{})
	okResults := 0

	chatPrompt := tools.NewChatPrompt().AddSystem(systemMessage)
	for _, msg := range p.History {
		chatPrompt.AddMessage(msg)
	}

retry:
	response, err := p.Client.RunRequest(&cmds.ClientRequest{
		ProcessName: p.ProcessName,
		GetCompletionRequests: tools.Replicate(cmds.GetCompletionRequest{
			RawPrompt:   chatPrompt.DefString(),
			Temperature: p.Temperature,
			MinResults:  minResults,
		}, minResults),
	}, 120*time.Second, executionPool)
	if err != nil {
		time.Sleep(100 * time.Millisecond)
		goto retry
	}

	choices := tools.DropDuplicates(tools.FlattenChoices(response.GetCompletionResponse))
	if len(choices) > minResults {
		minResults = len(choices) + 1
	}

	for _, choice := range choices {
		if _, exists := parsedChoices[choice]; exists {
			continue
		}
		parsedChoices[choice] = struct{}{}

		var parsedValue string
		if p.FullResultProcessor != nil {
			res, err := p.FullResultProcessor(choice)
			if err == nil && res == RPOProcessed {
				okResults++
				continue
			}
		}

		if len(p.ResultsProcessor) > 0 {
			var result = RPOIgnored
			err := tools.ParseJSON(choice, func(s string) error {
				for _, req := range p.ResponseFields {
					parsedValue = gjson.Get(s, req.Key).String()
					if parsedValue == "" {
						return fmt.Errorf("no value parsed")
					}

					if p.ResultsProcessor[req.Key] != nil {
						res, err := p.ResultsProcessor[req.Key](parsedValue)
						if err != nil && res == RPOFailed {
							return err
						}
						if res == RPOProcessed {
							result = res
							return nil
						}
					}
				}

				return nil
			})
			if err == nil && result == RPOProcessed {
				okResults++
			}
		}
	}

	if okResults < p.MinParsableResults {
		minResults++
		goto retry
	}

	return nil
}

func (p *SimplePipeline) WithHistory(history []*engines.Message) *SimplePipeline {
	p.History = history

	return p
}

func (p *SimplePipeline) WithUserMessage(desc string) *SimplePipeline {
	p.History = append(p.History, &engines.Message{
		Role:    engines.ChatRoleUser,
		Content: desc,
	})

	return p
}
