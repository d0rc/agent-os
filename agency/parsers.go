package agency

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/tools"
	"strings"

	//"gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
	"io"
	"reflect"
)

type AgentSettings struct {
	Agent *GeneralAgentSettings `yaml:"agent"`
}

type LifeCycleType string
type GeneralAgentSettings struct {
	Name                  string                    `yaml:"name"`
	InputSink             interface{}               `yaml:"input-sink"`
	PromptBased           *PromptBasedAgentSettings `yaml:"prompt-based"`
	LifeCycleType         LifeCycleType             `yaml:"life-cycle-type"`
	LifeCycleLength       int                       `yaml:"life-cycle-length"`
	renderedJson          string
	renderedJsonStructure []MapKV
}

type ResponseFormatType map[string]interface{}
type PromptBasedAgentSettings struct {
	Prompt          string             `yaml:"prompt"`
	Vars            map[string]any     `yaml:"vars"`
	ResponseFormat  ResponseFormatType `yaml:"response-format"`
	ResponseParsers []ResponseParser   `yaml:"response-parsers"`
}

type ResponseParser struct {
	ParserPath interface{} `yaml:"path"`
	ResultTags []string    `yaml:"tags"`
}

type ResponseParserResult struct {
	Tags   []string
	Value  interface{}
	Path   interface{}
	Parser *ResponseParser
}

func (rpr *ResponseParserResult) HasAnyTags(v ...string) bool {
	for _, tag := range v {
		for _, t := range rpr.Tags {
			if t == tag {
				return true
			}
		}
	}

	return false
}

func ParseAgency(data []byte) ([]*AgentSettings, error) {
	var settings []*AgentSettings

	err := yaml.Unmarshal(data, &settings)
	if err != nil {
		return nil, err
	}

	_, responseJson, responseJsonStructure, err := ParseYAML(data)
	//fmt.Printf("res: %v\n", res)
	//fmt.Printf("responseJson: %v\n", responseJson)

	for _, setting := range settings {
		fixMap(setting.Agent.PromptBased.ResponseFormat)
		fixMap(setting.Agent.PromptBased.ResponseFormat)
	}

	// save json renderings in settings
	for idx, setting := range settings {
		setting.Agent.renderedJson = responseJson[idx]
		setting.Agent.renderedJsonStructure = responseJsonStructure[idx]
	}
	return settings, nil
}

type Node struct {
	Name   string
	Type   string
	Parent string
}

func ParseYAML(data []byte) ([]Node, []string, [][]MapKV, error) {
	var nodes []Node

	decoder := yaml.NewDecoder(io.Reader(bytes.NewReader(data)))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, nil, nil, err
	}
	// ok, so let's dive into the yaml AST
	// until we find the response-format node

	ctx := &responseFormatJsonContext{
		parsingStarted:     false,
		processingFailed:   false,
		finalJson:          make([]string, 0),
		finalJsonStructure: make([][]MapKV, 0),
	}
	buildResponseFormatJson(&node, ctx)

	return nodes, ctx.finalJson, ctx.finalJsonStructure, nil
}

func (settings *AgentSettings) GetResponseJSONFormat() string {
	return settings.Agent.renderedJson
}

func (settings *AgentSettings) GetAgentInitialGoal() string {
	promptLines := strings.Split(settings.Agent.PromptBased.Prompt, "\n")
	return promptLines[0]
}

func fixMap(data map[string]interface{}) {
	for k, v := range data {
		switch v := v.(type) {
		case map[interface{}]interface{}:
			// Convert map[interface{}]interface{} to map[string]interface{}
			convertedData := make(map[string]interface{})
			for k, v := range v {
				convertedData[k.(string)] = v
			}
			data[k] = convertedData
		case map[string]interface{}:
			// If the value is a map, recursively fix it
			fixMap(v)
		case reflect.Type:
			// If the value is a reflect.Type, handle it appropriately
			data[k] = v.String()
		}
	}
}

func (settings *AgentSettings) ParseResponse(response string) ([]*ResponseParserResult, string, string, error) {
	if strings.HasSuffix(response, "},\n\t}\n}") {
		// replace suffix with "}\n}"
		response = response[:len(response)-7]
		response += "}\n\t}\n}"
	}
	if strings.HasSuffix(response, "},\n}") {
		// replace suffix with "}\n}"
		response = response[:len(response)-4]
		response += "}\n}"
	}
	// step one is parse JSON itself, according to the schema
	var parsedResponse ResponseFormatType
	var parsedString string
	err := tools.ParseJSON(response, func(response string) error {
		parsedString = response
		return json.Unmarshal([]byte(response), &parsedResponse)
	})
	if err != nil {
		return nil, "", "", err
	}

	parsedString = strings.TrimSpace(parsedString)

	if len(response)-len(parsedString) > 100 {
		// fmt.Printf("Parsed string: ```\n%s\n```\nOriginal len: %d, parsed len: %d\n",
		//	aurora.BrightGreen(parsedString),
		//	len(response),
		//	len(parsedString))
	}
	results := make([]*ResponseParserResult, 0)

	parsedStructure := make(map[string]interface{})
	// pick data according to configured parsers
	for _, parser := range settings.Agent.PromptBased.ResponseParsers {
		if _, ok := parser.ParserPath.(string); ok {
			// it's a string, so it should be simple, just pick it
			if obj := parsedResponse[parser.ParserPath.(string)]; obj != nil {
				results = append(results, &ResponseParserResult{
					Tags:   parser.ResultTags,
					Value:  obj,
					Path:   parser.ParserPath,
					Parser: &parser,
				})
				parsedStructure[parser.ParserPath.(string)] = obj
			}
		}

		if pathList, ok := parser.ParserPath.([]string); ok {
			// it's a list of strings, which are map[string]interface{} keys
			// let's dive into the map
			var obj interface{}
			obj = parsedResponse
			for _, path := range pathList {
				if obj == nil {
					break
				}

				if objMap, ok := obj.(map[string]interface{}); ok {
					obj = objMap[path]
				} else {
					obj = nil
				}
			}

			if obj != nil {
				results = append(results, &ResponseParserResult{
					Tags:   parser.ResultTags,
					Value:  obj,
					Path:   parser.ParserPath,
					Parser: &parser,
				})
			}
		}
	}

	collectedJsonStructure := make([]MapKV, 0)
	for _, el := range settings.Agent.renderedJsonStructure {
		if _, ok := el.Value.(string); ok {
			collectedJsonStructure = append(collectedJsonStructure, MapKV{
				Key:   el.Key,
				Value: parsedStructure[el.Key],
			})
			continue
		} else if el.InnerMap != nil {
			collectedJsonStructure = append(collectedJsonStructure, MapKV{
				Key:      el.Key,
				InnerMap: getUpdatedInnerMap(el.InnerMap, parsedStructure[el.Key]),
			})
			continue
		} else {
			fmt.Println("error, don't know how to handle this type: ", reflect.TypeOf(el.Value))
		}
	}
	//reconstructedParsedJson, _ := json.MarshalIndent(parsedStructure, "", "\t")
	reconstructedParsedJson := renderJsonString(collectedJsonStructure, &strings.Builder{}, 0)

	return results, parsedString, reconstructedParsedJson, nil
}

func getUpdatedInnerMap(innerMap []MapKV, parsed interface{}) []MapKV {
	if parsed == nil {
		return nil
	}
	if len(innerMap) == 0 {
		// we're at the bottom
		if parsedMap, ok := parsed.(map[string]interface{}); ok {
			newInnerMap := make([]MapKV, 0)
			for k, v := range parsedMap {
				newInnerMap = append(newInnerMap, MapKV{
					Key:   k,
					Value: v,
				})
			}
			return newInnerMap
		} else {
			fmt.Printf("don't know how to handle this type: %v\n", reflect.TypeOf(parsed))
		}
	}
	newInnerMap := make([]MapKV, 0)
	if parsedMap, ok := parsed.(map[string]interface{}); ok {
		for _, el := range innerMap {
			if _, ok := el.Value.(string); ok {
				parsedValue, ok := parsedMap[el.Key].(string)
				if ok {
					newInnerMap = append(newInnerMap, MapKV{
						Key:   el.Key,
						Value: parsedValue,
					})
				}
				continue
			} else if innerMapValue, ok := parsed.(map[string]interface{})[el.Key]; ok {
				newInnerMap = append(newInnerMap, MapKV{
					Key:      el.Key,
					InnerMap: getUpdatedInnerMap(el.InnerMap, innerMapValue),
				})
			} else {
				fmt.Printf("don't know how to handle this type: %v\n", reflect.TypeOf(el.Value))
			}
		}
	} else {
		fmt.Printf("don't know how to handle this type: %v\n", reflect.TypeOf(parsed))
	}
	return newInnerMap
}
