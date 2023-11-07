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
	queriesGenerator, err := agency.NewSimplePromptBasedGeneratingAgent(&agency.AgentConfig{
		PromptBased: &agency.PromptBasedAgentConfig{
			Prompt: fmt.Sprintf(`You are AI, your goal is to generate as many as possible google search keywords in order to get more understanding in the field of original goal: 
     
Original goal from team member: %s

First decide in which language to produce keywords in, and stick forever to that decision, this will make your results reliable. 

Before generation, think about nuances, such as in what language or set of languages to search for, what team could be missing in their request, but what it would be definitely interested in to achieve their goal. 


Make sure you search for relevant information, giving out too broad searches will cause harm to the team.


Don't worry, take a deep breath and think step by step. Remember it's your only and single purpose, so dig as deep as you can and shine as bright as possible...!`, goal),
			ResponseFormat: struct {
				Thoughts      string   `json:"thoughts"`
				Criticism     string   `json:"criticism"`
				Language      string   `json:"language"`
				Ideas         string   `json:"ideas"`
				SearchQueries []string `json:"search-queries"`
			}{
				Thoughts:      "place your thoughts here",
				Criticism:     "constructive self-criticism",
				Language:      "thoughts on languages to produce keywords in",
				Ideas:         "thoughts on search ideas to make results perfect",
				SearchQueries: []string{"search queries in the language you've chosen"},
			},
			LifeCycleType:   agency.LifeCycleSingleShot,
			LifeCycleLength: 200,
		},
	})

	if err != nil {
		zlog.Fatal().Err(err).Msg("error creating agent...!")
	}

	fmt.Printf("agent's prompt:\n%s\n", aurora.White(queriesGenerator.GeneratePrompt()))
}
