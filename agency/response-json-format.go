package agency

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strings"
)

type responseFormatJsonContext struct {
	parsingStarted     bool
	processingFailed   bool
	finalJson          []string
	finalJsonStructure [][]MapKV
}

func buildResponseFormatJson(node *yaml.Node, ctx *responseFormatJsonContext) {
	if ctx.processingFailed {
		return
	}
	if !ctx.parsingStarted {
		if node.Value == "response-format" {
			ctx.parsingStarted = true
			// next token to this is response format definition
			return
		} else {
			if node.Content != nil {
				for i := range node.Content {
					buildResponseFormatJson(node.Content[i], ctx)
					if ctx.parsingStarted {
						// we've found the response-format node, so we can stop
						if len(node.Content) <= i+1 {
							ctx.processingFailed = true
						} else {
							responseStructure := tryToCollectJsonString(node.Content[i+1], ctx)
							jsonString := RenderJsonString(responseStructure, &strings.Builder{}, 0)
							//fmt.Printf("responseStructure: %v\n", jsonString)
							ctx.finalJson = append(ctx.finalJson, jsonString)
							ctx.finalJsonStructure = append(ctx.finalJsonStructure, responseStructure)
							return
						}
						return
					}
					if ctx.processingFailed {
						return
					}
				}
			}
		}
	} else {

	}
}

func RenderJsonString(structure []MapKV, buffer *strings.Builder, depth int) string {
	if depth == 0 {
		buffer.WriteString("{\n")
		RenderJsonString(structure, buffer, depth+1)
		buffer.WriteString("}\n")
	} else {
		for _, kv := range structure {
			if kv.Value != nil {
				buffer.WriteString(fmt.Sprintf("%s\"%s\": \"%s\",\n", strings.Repeat("\t", depth), kv.Key, kv.Value))
			} else {
				buffer.WriteString(fmt.Sprintf("%s\"%s\": {\n", strings.Repeat("\t", depth), kv.Key))
				RenderJsonString(kv.InnerMap, buffer, depth+1)
				buffer.WriteString(fmt.Sprintf("%s},\n", strings.Repeat("\t", depth)))
			}
		}
	}

	return buffer.String()
}

type MapKV struct {
	Key      string
	Value    interface{}
	InnerMap []MapKV
}

func tryToCollectJsonString(node *yaml.Node, ctx *responseFormatJsonContext) []MapKV {
	if node.Tag == "!!map" {
		mapData := make([]MapKV, 0)
		currentKV := &MapKV{}
		for idx, contentNode := range node.Content {
			if contentNode.Tag == "!!str" && idx%2 == 0 {
				// it seems to be map key
				currentKV = &MapKV{Key: contentNode.Value}
			}
			if idx%2 == 1 {
				if contentNode.Tag == "!!str" {
					currentKV.Value = contentNode.Value

					mapData = append(mapData, *currentKV)
				}
				if contentNode.Tag == "!!map" {
					// dive into map
					currentKV.InnerMap = tryToCollectJsonString(contentNode, ctx)
					mapData = append(mapData, *currentKV)
				}
			}
		}

		return mapData
	}

	ctx.processingFailed = true
	// fmt.Printf("failed to parse response-format: %v, not a map received...!\n", node)
	return nil
}
