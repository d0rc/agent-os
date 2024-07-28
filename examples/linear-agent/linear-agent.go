package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/agency"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/stdlib/generics"
	os_client "github.com/d0rc/agent-os/stdlib/os-client"
	"github.com/d0rc/agent-os/stdlib/tools"
	"github.com/d0rc/agent-os/syslib/utils"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora"
	zlog "github.com/rs/zerolog/log"
	"os"
	"strings"
	"time"
)

var agentOSUrl = flag.String("agent-os-url", "http://127.0.0.1:9000", "agent-os endpoint")
var termUi = false
var config = flag.String("agency-config", "../work2/agency.yaml", "path to agency config")
var goal = flag.String("goal", "Create a detailed report covering what is the monthly rent price for 4xA100 80Gb server with 384 GB RAM? Can I get one with crypto? Keep track of all URLs you use as references so I could double check your report.", "override goal provided in agency.yaml")

func main() {
	ts := time.Now()
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msg("starting research-agency-1")

	agencyYaml, err := os.ReadFile(*config)
	if err != nil {
		lg.Fatal().Err(err).Msgf("failed to read agency config, path = `%s`", *config)
	}

	agencySettings, err := agency.ParseAgency(agencyYaml)
	if err != nil {
		lg.Fatal().Err(err).Msg("failed to parse agency")
	}
	if goal != nil && *goal != "" {
		agencySettings[0].Agent.PromptBased.Vars["goal"] = *goal
	}

	lg.Info().Interface("agencySettings", agencySettings).Msg("parsed agency")

	client := os_client.NewAgentOSClient(*agentOSUrl)
	agentState := agency.NewGeneralAgentState(client, "", agencySettings[0])

	systemGoal := agentState.GetSystemGoal()

	systemPrompt, err := agentState.GetSystemMessageWithVars(map[string]string{"history": "No history yet."})
	if err != nil {
		lg.Fatal().Msgf("Failed to get system message: %s", err)
	}

	//fmt.Printf("Rendered system prompt: \n%s\n", aurora.White(systemPrompt.Content))

	processName := "single-agent"

	stepNo := 0
	var historicalContext string = ""
	history := make([]*engines.Message, 0)
	for {
		stepNo++
		if len(history) > 7 {
			// context now is too long, so we should summarize our activity now
			historicalContext = processHistory(client, processName, systemPrompt.Content, history, systemGoal, historicalContext)
			systemPrompt, err = agentState.GetSystemMessageWithVars(map[string]string{"history": historicalContext})
			history = make([]*engines.Message, 0)
			if err != nil {
				lg.Fatal().Msgf("Failed to get system message: %s", err)
			}
		}
		parsed, rawContent, err := makeAgentStep(client, processName, systemPrompt.Content, history)
		if err != nil {
			lg.Fatal().Msgf("Failed to make a step: %v", err)
		}

		history = append(history, &engines.Message{
			Role:    engines.ChatRoleAssistant,
			Content: rawContent,
		})

		showAgentResponse(parsed)

		v := parsed["command"]
		trx := uuid.New().String()
		commandName, okCommandName := v.(map[string]interface{})["name"].(string)
		argsData, okArgsData := v.(map[string]interface{})["args"].(map[string]interface{})
		clientRequests := make([]*cmds.ClientRequest, 0)
		if okCommandName && okArgsData {
			clientRequests = append(clientRequests,
				agentState.GetServerCommand(
					trx,
					commandName,
					argsData,
					func(a, b string) {
						fmt.Printf("Got reactive data: %s, %s\n", a, b)
					})...)
		}

		//fmt.Printf("Got following commands to execute: %v\n", clientRequests)

		responses, err := client.RunRequests(clientRequests, 120*time.Second)
		if err != nil {
			lg.Fatal().Msgf("Got bad response from server: %v", err)
		}

		//fmt.Printf("Got the following responses: %v\n", responses)
		//fmt.Printf("First response: %v\n", responses[0])

		observations := agency.GenerateObservationFromServerResults(clientRequests[0], responses[0], 4096, agentState)
		if len(observations) > 0 {
			fmt.Printf("Got the following observations:\n%s\n", aurora.BrightCyan(observations[0]))
			history = append(history, &engines.Message{
				Role:    engines.ChatRoleUser,
				Content: fmt.Sprintf("Command `%s` output:\n%s\n", commandName, strings.Join(observations, "\n")),
			})
		} else {
			history = append(history, &engines.Message{
				Role:    engines.ChatRoleUser,
				Content: "command finished without error, no more data to show",
			})
		}

		fmt.Printf("Done step %d in %v.", stepNo, time.Since(ts))

		if commandName == "final-report" {
			fmt.Printf("============== Final report ===============\n\n%s\n", argsData["text"])
			break
		}
	}
}

func processHistory(client *os_client.AgentOSClient, processName string, systemPrompt string, history []*engines.Message, goal, previousSummary string) string {
	history = append(history, &engines.Message{Role: engines.ChatRoleUser, Content: fmt.Sprintf(`Goal description: %s

Please generate a summary of the recent interactions and historical context, provided in "Your history:" clause; summary should be based on the goal description and log of operations. Keep track of the commands issued, their responses, and any significant data or conclusions. Format the summary as follows:


# Context history and summary

## Sites visited
- ...
- ...
- ...

## Searches executed
- ...
- ...
- ...

## A bullet list of key takeaways and important facts
* ...
* ...
* ...

## Contradictions and doubtful facts
* ...
* ...
* ...

# Conclusion
...


Always provide summaries in markdown format.
`, goal)})

	pipeline := generics.CreateSimplePipeline(client, processName).
		//WithSystemMessage(systemPrompt).
		WithMinParsableResults(1).
		WithMaxParsableResults(1)

	for _, hr := range history {
		if hr.Role == engines.ChatRoleAssistant {
			pipeline = pipeline.WithAssistantMessage(hr.Content)
		}
		if hr.Role == engines.ChatRoleUser {
			pipeline = pipeline.WithUserMessage(hr.Content)
		}
	}

	var rawContent string
	err := pipeline.WithRawResultsProcessor(func(rawResult string) error {
		rawContent = rawResult
		return nil
	}).Run(os_client.REP_Default)

	if err != nil {
		zlog.Fatal().Msgf("Failed to make a step: %v", err)
	}

	fmt.Printf("Summary:\n\n%s\n", aurora.BrightBlue(rawContent))
	fmt.Printf("Previous summary: \n\n%s\n", aurora.BrightCyan(previousSummary))

	// Now it's the trickiest part. We need to summarize the summaries.
	if previousSummary == "" {
		return rawContent
	}

	finalSummary, err := mergeSummaries(client, processName, previousSummary, rawContent, goal)

	fmt.Printf("Merged summary:\n\n%s\n", aurora.BrightYellow(finalSummary))

	return finalSummary
}

func mergeSummaries(client *os_client.AgentOSClient, processName string, summary string, content string, goal string) (string, error) {
	var finalResult string = ""
	err := generics.CreateSimplePipeline(client, processName).
		WithSystemMessage(fmt.Sprintf(`I have two summaries that describe the history of solving a particular task over two different time periods. These summaries may contain similar actions with different results, akin to work logs. Can you help me combine them into one cohesive summary that highlights the key actions, the evolution of the process, and the differing outcomes between these periods?

Summary 1:

%s


Summary 2:

%s

Your ultimate goal: %s

Provide a combined summary in markdown format, ensure to highlight results and bits of information reuiqred to satisfy the goal.
`, mdQuote(summary), mdQuote(content), goal)).
		WithRawResultsProcessor(func(rawResults string) error {
			finalResult = rawResults
			return nil
		}).
		Run(os_client.REP_Default)

	return finalResult, err
}

func mdQuote(s string) string {
	return fmt.Sprintf("\n```\n%s\n```\n", s)
}

func showAgentResponse(parsed map[string]interface{}) {
	fmt.Printf("==================================================\nThoughts: %s\nCriticism: %s\nCommand: %v\nArgs: %v\n",
		aurora.BrightGreen(parsed["thoughts"]),
		aurora.BrightYellow(parsed["criticism"]),
		aurora.BrightBlue(parsed["command"].(map[string]interface{})["name"]),
		aurora.BrightCyan(parsed["command"].(map[string]interface{})["args"]))
}

func makeAgentStep(client *os_client.AgentOSClient, processName, systemPrompt string, history []*engines.Message) (map[string]interface{}, string, error) {
	parsedRespChan := make(chan map[string]interface{}, 10)
	rawRespChan := make(chan string, 10)
	pipeline := generics.CreateSimplePipeline(client, processName).
		WithSystemMessage(systemPrompt).
		WithMinParsableResults(1).
		WithMaxParsableResults(1)

	for _, hr := range history {
		if hr.Role == engines.ChatRoleAssistant {
			pipeline = pipeline.WithAssistantMessage(hr.Content)
		}
		if hr.Role == engines.ChatRoleUser {
			pipeline = pipeline.WithUserMessage(hr.Content)
		}
		if hr.Role == engines.ChatRoleSystem {
			pipeline = pipeline.WithSystemMessage(hr.Content)
		}
	}

	var exitError error = nil
	err := pipeline.
		WithResultsProcessor(func(results map[string]interface{}, rawResponse string) error {
			if rawResponse == "context overflow" {
				fmt.Printf("=================\n%s\n%s\n", systemPrompt, tools.NewChatPromptWithMessages(history))
				exitError = fmt.Errorf("context overflow error")
			}
			if len(results) == 0 {
				return fmt.Errorf("no parsable response")
			}
			parsedRespChan <- results
			rawRespChan <- rawResponse
			return nil
		}).Run(os_client.REP_Default)

	if err == nil {
		err = exitError
	}

	return <-parsedRespChan, <-rawRespChan, err
}
