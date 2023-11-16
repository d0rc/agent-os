# Developing with AgencyOS

`agency.yaml` - is a YAML definition of the agency, which is a list agents, each of which is whatever you'd like, but I think it's computational procedure, which is described using:

- Initial prompt;
- Initial context (map of variables which are passed to templating engine with initial prompt);
- Response format.

Now, if you run this:

```bash
go run ./examples/who-is-jimmy-apples/who-is-jimmy-apples.go -n 12
```

You will see something like this:

```javascript

--- many many lines here ---

End of the list of the most suggested search queries:
65. Background of Jimmy Apples - 160
66. Jimmy Apples controversies - 160
67. Jimmy Apples education - 192
68. Jimmy Apples social media profiles - 288
69. Jimmy Apples personal life - 288
70. Jimmy Apples background - 480
71. Jimmy Apples bio - 544
72. Jimmy Apples biography - 576
73. Jimmy Apples achievements - 576
74. Jimmy Apples net worth - 800
75. Jimmy Apples career - 800
76. Who is Jimmy Apples? - 832
Done - total results: 1312
Execution finished in  1.512428667s
```

Which is top 12 search queries, proposed by the agent from the description for the goal, defined in binary.

You may notice the first run is slow and consequent runs are fast, that is because extensive caching applied for results of all compute or IO-heavy actions. No result is ever lost, unless you want to, if you still want to get some more inferences on each run, you can request a different number of results from the system and it will generate new ones, while still serving the old ones.

## running inference


```golang
	results, err := agency.GeneralAgentPipelineStep(agentState,
		0,   // current depth of history, 0 - means only system prompt
    32,  // try to create this many inference jobs
		100, // how many times we can try to sample `batchSize` jobs
		40,  // how many inference results before using only cached inference
		agentContext)
```

The whole point is to allow underlying engine "explore" as many possible paths as it can, which in turn is beneficial in many senses, to name a few - production setting, when solution has to be found, fine-tuning data set preparations, internal stability benchmarks, etc. 

## parsing responses

The code explains it better, this what is done to results from the previous chapter in the example code:

```golang
suggestedSearches := make(map[string]int)
for _, res := range results {
    parsedResults, _ := agentState.ParseResponse(res.Content)
    for _, parsedResult := range parsedResults {
       if parsedResult.HasAnyTags("thoughts") {
          // let's print thoughts for fun and debug...!
          fmt.Printf("thoughts: %s\n", aurora.BrightWhite(parsedResult.Value))
       }
       if parsedResult.HasAnyTags("search-queries") {
          listOfStrings := parsedResult.Value.([]interface{})
          for _, v := range listOfStrings {
             // notify user about new search query parsed...!
             fmt.Printf("search query: %s\n", aurora.BrightYellow(v))
             suggestedSearches[v.(string)]++
          }
       }
    }
}
```

