package agency

import (
	"github.com/d0rc/agent-os/tools"
	"gopkg.in/yaml.v3"
	"strings"
)

type responseFormatJsonContext struct {
	parsingStarted     bool
	processingFailed   bool
	finalJson          []string
	finalJsonStructure [][]tools.MapKV
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
							jsonString := tools.RenderJsonString(responseStructure, &strings.Builder{}, 0)
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

func tryToCollectJsonString(node *yaml.Node, ctx *responseFormatJsonContext) []tools.MapKV {
	if node.Tag == "!!map" {
		mapData := make([]tools.MapKV, 0)
		currentKV := &tools.MapKV{}
		for idx, contentNode := range node.Content {
			if contentNode.Tag == "!!str" && idx%2 == 0 {
				// it seems to be map key
				currentKV = &tools.MapKV{Key: contentNode.Value}
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
