package gaba

import (
	"fmt"
	"github.com/d0rc/agent-os/stdlib/generics"
	os_client "github.com/d0rc/agent-os/stdlib/os-client"
	"sync"
)

type HallucinationStat struct {
	SrcTerms       int
	DstTerms       int
	DstTermsNew    int
	DstTermsCommon int
}

func EstimateHallucinations(client *os_client.AgentOSClient, source, destination string) *HallucinationStat {
	sourceTerms := make([]string, 0)
	destinationTerms := make([]string, 0)
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		sourceTerms = ExtractNamedEntities(client, source)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		destinationTerms = ExtractNamedEntities(client, destination)
		wg.Done()
	}()

	wg.Wait()

	commonTerms := make(map[string]struct{})
	for _, srcTerm := range sourceTerms {
		for _, dstTerm := range destinationTerms {
			if srcTerm == dstTerm {
				commonTerms[srcTerm] = struct{}{}
			}
		}
	}

	newTerms := make(map[string]struct{})
	for _, dstTerm := range destinationTerms {
		if _, ok := commonTerms[dstTerm]; !ok {
			newTerms[dstTerm] = struct{}{}
		}
	}

	if len(newTerms) < 2*len(sourceTerms) {
		fmt.Printf("[OK] Got new terms: %v\n", newTerms)
	} else {
		fmt.Printf("[ERROR] Got new terms: %v\n", newTerms)
	}

	return &HallucinationStat{
		SrcTerms:       len(sourceTerms),
		DstTerms:       len(destinationTerms),
		DstTermsNew:    len(newTerms),
		DstTermsCommon: len(commonTerms),
	}
}

func ExtractNamedEntities(client *os_client.AgentOSClient, source string) []string {
	collectedNames := make(map[string]struct{})
	_ = generics.CreateSimplePipeline(client, "ner-extraction").
		WithSystemMessage("For each user message output a JSON with a list of named entities in the user message.").
		WithUserMessage(source).
		WithMinParsableResults(1).
		WithResultsProcessor(func(res map[string]interface{}, choice string) error {
			if res != nil {
				for _, entities := range res {
					if entitiesList, ok := entities.([]interface{}); ok {
						for _, entity := range entitiesList {
							if entityMap, ok := entity.(map[string]interface{}); ok {
								if name, exists := entityMap["name"]; exists {
									if nameStr, ok := name.(string); ok {
										collectedNames[nameStr] = struct{}{}
									}
								}
							} else if entityName, ok := entity.(string); ok {
								collectedNames[entityName] = struct{}{}
							}
						}
					}

					return nil
				}
			}

			return fmt.Errorf("error processing results")
		}).Run(os_client.REP_IO)

	names := make([]string, 0, len(collectedNames))
	for name := range collectedNames {
		names = append(names, name)
	}

	return names
}
