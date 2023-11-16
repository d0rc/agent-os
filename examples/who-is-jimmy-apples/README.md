# Developing with AgencyOS

`agency.yaml` is a YAML file that defines the agency. The agency is comprised of a list of agents, and each agent can be any computational procedure you choose. The definition includes:

- An initial prompt;
- An initial context, which is a map of variables passed to the templating engine along with the initial prompt;
- A response format.

When you execute the following command:

```bash
go run ./examples/who-is-jimmy-apples/who-is-jimmy-apples.go -n 12
```

You'll receive output similar to this:

```javascript
--- many lines of output ---

End of the list of the most suggested search queries:
65. Background of Jimmy Apples - 160
...
76. Who is Jimmy Apples? - 832
Done - total results: 1312
Execution finished in  1.512428667s
```

This output represents the top 12 search queries suggested by the agent based on the description for the goal, as defined in binary.

You may notice that the first execution is slower, but subsequent runs are quicker due to extensive caching of results from compute or IO-intensive actions. The system retains all results unless deliberately discarded. To obtain additional inferences in each run, you can request a different number of results, and the system will generate new ones while still serving the old.

## Running Inference

The following Golang code snippet illustrates how to run inference:

```golang
	results, err := agency.GeneralAgentPipelineStep(agentState,
		0,   // current depth of history, 0 - means only system prompt
    32,  // number of inference jobs to create
		100, // max attempts to sample `batchSize` jobs
		40,  // number of inference results before using only cached inference
		agentContext)
```

The objective is to enable the underlying engine to explore as many paths as possible. This approach is beneficial in several contexts, including production environments where solutions must be found, preparation of fine-tuning datasets, and internal stability benchmarks.

## Parsing Responses

The example code below shows how to process the results obtained in the previous step:

```golang
suggestedSearches := make(map[string]int)
for _, res := range results {
    parsedResults, _ := agentState.ParseResponse(res.Content)
    for _, parsedResult := range parsedResults {
       if parsedResult.HasAnyTags("thoughts") {
          // Outputting thoughts for fun and debugging...
          fmt.Printf("thoughts: %s\n", aurora.BrightWhite(parsedResult.Value))
       }
       if parsedResult.HasAnyTags("search-queries") {
          listOfStrings := parsedResult.Value.([]interface{})
          for _, v := range listOfStrings {
             // Informing user about new search query parsed...
             fmt.Printf("search query: %s\n", aurora.BrightYellow(v))
             suggestedSearches[v.(string)]++
          }
       }
    }
}
```

This code processes the results, counting suggested search queries.