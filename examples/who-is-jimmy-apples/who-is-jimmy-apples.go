package main

import (
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/logrusorgru/aurora"
	zlog "github.com/rs/zerolog/log"
)

/*

	this is an example of low-level tool for agent-os, it's purpose is to create a report
	answering the question `who is Jimmy Apples?`

*/

const goal = "Figure out, who is Jimmy Apples."

func main() {
	// first, try to search internet
	// we're passing literal as a configuration, but it's the same as YAML
	// yes, agents _can_ develop agents
	queriesGenerator, err := agency.NewSimplePromptBasedGeneratingAgent(&agency.AgentConfig{
		PromptBased: &agency.PromptBasedAgentConfig{
			Prompt: fmt.Sprintf(`You are AI, your goal is to generate as many as possible google search keywords in order to get more understanding in the field of original goal: 
     
Original goal from team member: %s

First decide in which language to produce keywords in, and stick forever to that decision, this will make your results reliable. 

Before generation, think about nuances, such as in what language or set of languages to search for, what team could be missing in their request, but what it would be definitely interested in to achieve their goal. 


Make sure you search for relevant information, giving out too broad searches will cause harm to the team.


Don't worry, take a deep breath and think step by step. Remember it's your only and a single purpose, so dig as deep as you can and shine as bright as possible...!`, goal),
			ResponseFormat: map[string]interface{}{
				"thoughts":       "place your thoughts here",
				"criticism":      "constructive self-criticism",
				"language":       "thoughts on languages to produce keywords in",
				"ideas":          "thoughts on search ideas to make results perfect",
				"search-queries": []string{"search queries in the language you've chosen"},
			},
			LifeCycleType:   agency.LifeCycleSingleShot,
			LifeCycleLength: 200,
		},
	})
	if err != nil {
		zlog.Fatal().Err(err).Msg("error creating agent...!")
	}

	fmt.Printf("agent's prompt:\n%s\n", aurora.White(queriesGenerator.GeneratePrompt()))

	/*
		results, err := env.Run(queriesGenerator, func(resp map[string]interface{}, results chan interface{}) {
			data := resp["search-queries"]
			searchQueriesSlice, ok := data.([]string)
			if ok {
				for _, q := range searchQueriesSlice {
					results <- q
				}
			}
		})
		if err != nil {
			zlog.Fatal().Err(err).Msg("error running agent")
		}

		for _, agentResult := range results {
			env.ExecuteGoogleSearch(agentResult)
		}*/
}
