package main

import (
	_ "embed"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/utils"
	"github.com/logrusorgru/aurora"
	"sort"
	"time"
)

/*

	this is an example of low-level tool for agent-os, it's purpose is to create a report
	answering the question `who is Jimmy Apples?`

*/

var nQueriesToShow = flag.Int("n", 10, "number of queries to show")
var goal = flag.String("goal", "Figure out, who is Jimmy Apples.", "goal to achieve")

//go:embed agency.yaml
var agencyYaml []byte

var termUi = false

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting who-is-jimmy-apples")

	agencySettings, err := agency.ParseAgency(agencyYaml)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to parse agency")
	}

	lg.Info().Interface("agencySettings", agencySettings).Msg("parsed agency")

	client := os_client.NewAgentOSClient("http://localhost:9000")
	agentState := agency.NewGeneralAgentState(client, "", agencySettings[0])
	agentContext := &agency.InferenceContext{
		InputVariables: map[string]any{"goal": *goal},
		History:        make([][]*engines.Message, 0),
	}
	results, err := agency.GeneralAgentPipelineStep(agentState,
		0,   // currentDepth
		32,  // batchSize
		100, // maxSamplingAttempts
		40,  // minResults
		agentContext)

	if err != nil {
		lg.Error().Err(err).
			Interface("results", results).
			Msg("failed to run inference")
	} else {
		suggestedSearches := make(map[string]int)
		for _, res := range results {
			parsedResults, _ := agentState.ParseResponse(res.Content)
			for _, parsedResult := range parsedResults {
				if parsedResult.HasAnyTags("thoughts") {
					fmt.Printf("thoughts: %s\n", aurora.BrightWhite(parsedResult.Value))
				}
				if parsedResult.HasAnyTags("search-queries") {
					listOfStrings := parsedResult.Value.([]interface{})
					for _, v := range listOfStrings {
						if v == nil {
							continue
						}
						fmt.Printf("search query: %s\n", aurora.BrightYellow(v))
						suggestedSearches[v.(string)]++
					}
				}
			}
		}

		fmt.Println("End of the list of the most suggested search queries:")
		PrintLeaderBoardTable(suggestedSearches, *nQueriesToShow)
		fmt.Println("Done - total results:", len(results))
	}
	fmt.Println("Execution finished in ", time.Since(ts))
}

func PrintLeaderBoardTable(leaderboardData map[string]int, n int) {
	// sort searches by value, lowest - first
	type kv struct {
		Key   string
		Value int
	}

	var leaderBoardSlice []kv
	for k, v := range leaderboardData {
		leaderBoardSlice = append(leaderBoardSlice, kv{k, v})
	}

	// now sort by value, using standard library
	sort.Slice(leaderBoardSlice, func(i, j int) bool {
		return leaderBoardSlice[i].Value < leaderBoardSlice[j].Value
	})

	if n == -1 || n > len(leaderBoardSlice) {
		n = len(leaderBoardSlice)
	}

	// now print last n elements
	for i := len(leaderBoardSlice) - n; i < len(leaderBoardSlice); i++ {
		fmt.Printf("%d. %s - %d\n", i+1, aurora.BrightWhite(leaderBoardSlice[i].Key), aurora.BrightWhite(leaderBoardSlice[i].Value))
	}

	return
}
