package generics

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	agent_tools "github.com/d0rc/agent-os/stdlib/agent-tools"
	"github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/flosch/pongo2/v6"
	"github.com/logrusorgru/aurora"
	"strings"
	"sync/atomic"
	"time"
)

type ResultProcessingOutcome int

type SimplePipeline struct {
	SystemMessage               string
	Vars                        map[string]interface{}
	Temperature                 float32
	AssistantResponsePrefix     map[int]string
	ResponseFields              []tools.MapKV
	MinParsableResults          int
	Client                      *os_client.AgentOSClient
	ProcessName                 string
	Tools                       []agent_tools.AgentTool
	History                     []*engines.Message
	FullParsedResponseProcessor func(map[string]interface{}, string) error
}

func CreateSimplePipeline(client *os_client.AgentOSClient, name string) *SimplePipeline {
	result := &SimplePipeline{
		Vars:                    make(map[string]interface{}),
		AssistantResponsePrefix: make(map[int]string),
		ResponseFields:          make([]tools.MapKV, 0),
		MinParsableResults:      2,
		Temperature:             0.1,
		Client:                  client,
		ProcessName:             name,
		FullParsedResponseProcessor: func(m map[string]interface{}, s string) error {
			return nil
		},
	}

	result.Vars["name"] = name

	return result
}

func (p *SimplePipeline) WithSystemMessage(systemMessage string) *SimplePipeline {
	p.SystemMessage = systemMessage
	return p
}

func (p *SimplePipeline) WithResultsProcessor(processor func(map[string]interface{}, string) error) *SimplePipeline {
	p.FullParsedResponseProcessor = processor
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

func (p *SimplePipeline) WithTools(tools ...agent_tools.AgentTool) *SimplePipeline {
	p.Tools = tools
	return p
}

var symbols = []string{
	" | ",
	" / ",
	" - ",
	" \\ ",
	" / ",
	" - ",
	" \\ ",
}
var symbolIndex = uint64(0)

func nextSymbol() string {
	return symbols[int(atomic.AddUint64(&symbolIndex, 1)%uint64(len(symbols)))]
}

func (p *SimplePipeline) Run(executionPool os_client.RequestExecutionPool) error {
	minResults := p.MinParsableResults
	parsedChoices := make(map[string]struct{})
	okResults := 0

	done := make(chan struct{})
	cycle := 0
	defer func() {
		done <- struct{}{}
	}()

	go func() {
		for {
			select {
			case <-done:
				fmt.Printf("\r\033[K\r")
				return
			case <-time.After(time.Second * 1):
				fmt.Printf("\r\033[K\r[%s] %s is thinking, current cycle: %d [%d/%d]\r",
					nextSymbol(),
					aurora.BrightCyan(p.ProcessName),
					cycle,
					okResults,
					minResults)
			}
		}
	}()

	var systemMessage string
	var err error
retry:
	cycle++

	systemMessage, err = p.constructSystemMessage()
	if err != nil {
		return err
	}
	if cycle == 0 {
		minResults = p.MinParsableResults
	}

	chatPrompt := tools.NewChatPrompt().AddSystem(systemMessage)
	for _, msg := range p.History {
		chatPrompt.AddMessage(msg)
	}

	response := p.Client.RunRequest(&cmds.ClientRequest{
		ProcessName: p.ProcessName,
		GetCompletionRequests: tools.Replicate(cmds.GetCompletionRequest{
			RawPrompt:   chatPrompt.DefString(),
			Temperature: p.Temperature,
			MinResults:  minResults,
		}, min(64, minResults)),
	}, 120*time.Second, executionPool)

	choices := tools.DropDuplicates(tools.FlattenChoices(response.GetCompletionResponse))
	if len(choices) > minResults {
		minResults = len(choices) + 1
	}

	for _, choice := range choices {
		if _, exists := parsedChoices[choice]; exists {
			minResults++
			continue
		}
		parsedChoices[choice] = struct{}{}

		parsedResponse := make(map[string]interface{})
		var parsedValue string
		if err = tools.ParseJSON(choice, func(s string) error {
			parsedValue = s
			return json.Unmarshal([]byte(s), &parsedResponse)
		}); err != nil {
			// ...
		}

		if err = p.FullParsedResponseProcessor(parsedResponse, parsedValue); err != nil {
			// got error, processing response...!
		} else {
			okResults++
		}
	}

	if okResults < p.MinParsableResults {
		minResults++
		goto retry
	}

	return nil
}

func (p *SimplePipeline) constructSystemMessage() (string, error) {
	var systemMessage = p.SystemMessage

oneMoreSystemMessageFold:
	tpl, err := pongo2.FromString(systemMessage)
	if err != nil {
		return "", err
	}

	jsonBuffer := &strings.Builder{}
	tools.RenderJsonString(p.ResponseFields, jsonBuffer, 0)

	p.Vars["timestamp"] = fmt.Sprintf("%v", time.Now())

	systemMessage, err = tpl.Execute(p.Vars)
	if err != nil {
		return "", err
	}

	if strings.Contains(systemMessage, "{{") && strings.Contains(systemMessage, "}}") {
		goto oneMoreSystemMessageFold
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
		systemMessage = systemMessage + fmt.Sprintf("\nRespond in the following JSON format:\n```json\n%s```\n",
			jsonBuffer.String())
	}
	return systemMessage, nil
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

func (p *SimplePipeline) ConditionalField(flag bool, f func(sp *SimplePipeline) *SimplePipeline) *SimplePipeline {
	if flag {
		return f(p)
	}

	return p
}
