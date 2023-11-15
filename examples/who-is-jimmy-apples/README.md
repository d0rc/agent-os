When running this example, you will see the following output:


```bash
...
... (some output omitted)
...

{
  "criticism": "In order to provide a more focused and effective response, I suggest considering specific details about Jimmy Apples that could help narrow down the search queries.",
  "language": "English",
  "search-queries": [
    "Jimmy Apples biography",
    "Who is Jimmy Apples?",
    "Jimmy Apples background",
    "Jimmy Apples profession",
    "Jimmy Apples achievements",
    "Jimmy Apples net worth"
  ],
  "thoughts": "It would be helpful to have more information about the context surrounding Jimmy Apples, such as their occupation or a specific field they are associated with. This would allow for more targeted and relevant search queries."
}

Done - total results: 1312
```


When running multiple times, consequent results will the same, as long as you keep the settings:


```golang
	results, err := agency.GeneralAgentPipelineStep(agentState,
		0,   // currentDepth
		32,  // batchSize
		100, // maxSamplingAttempts
		40,  // minResults
		agentContext)
```


... and this is what it's intended to be:


```golang
func GeneralAgentPipelineStep(state *GeneralAgentInfo,
	currentDepth, // current depth of history, 0 - means only system prompt
	batchSize, // try to create this many jobs
	maxSamplingAttempts, // how many times we can try to sample `batchSize` jobs
	minResults int, // how many inference results before using only cached inference
	history *InferenceContext) ([]*engines.Message, error) {
```

the whole point is to allow underlying engine "explore" as many possible paths as it can, which in turn is beneficial in many senses, to name a few - production setting, when solution has to be found, fine-tuning data set preparations, internal stability benchmarks, etc. 

 