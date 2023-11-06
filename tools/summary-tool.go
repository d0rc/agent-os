package tools

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/utils"
	"github.com/logrusorgru/aurora"
	zlog "github.com/rs/zerolog/log"
	"strings"
	"time"
)

func DocumentReduceGetCached(document, question string, storage *cmds.Storage) (string, bool) {
	if len(document) == 0 {
		return "", true
	}

	cachedResult, err := storage.GetTaskCachedResult("document-reduce-v1", fmt.Sprintf("%s\n%s",
		document, question))
	if err == nil && cachedResult != nil && len(cachedResult) > 10 {
		return string(cachedResult), true
	}

	return "", false
}

func DocumentReduce(document, question string, storage *cmds.Storage, parser func(string) error, model string) string {
	if len(document) == 0 {
		return ""
	}

	cachedResult, err := storage.GetTaskCachedResult("document-reduce-v1", fmt.Sprintf("%s\n%s",
		document, question))
	if err == nil && cachedResult != nil && len(cachedResult) > 10 {
		return string(cachedResult)
	}

	ts := time.Now()
	systemPrompt := fmt.Sprintf(`You are an AI that seeks to answer the following question:\n%s`, question)

	snippets, err := tokenizeDocument(document, storage)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}

	success := false
	currentSummary := ""
	for idx, snippet := range snippets {
		minResults := 1
		retryCounter := 0
	retryGeneratingSummary:
		retryCounter++
		tmp, err := cmds.ProcessGetCompletions([]cmds.GetCompletionRequest{
			{
				RawPrompt: fmt.Sprintf(`
### System: %s
### User: Source data:

%s

%s

### Assistant:
`, systemPrompt, strings.TrimSpace(snippet), strings.TrimSpace(strings.TrimPrefix(currentSummary, "Source data:"))),
				Model:       model,
				Temperature: 0.8,
				StopTokens:  []string{"###"},
				MinResults:  minResults,
				BestOf:      3,
				MaxResults:  1,
			},
		}, storage)
		minResults = 100 // next time we want all results, when making retry

		if err != nil {
			zlog.Error().
				Err(err).
				// Str("question", question).
				Int("snippet_idx", idx).
				Msg("failed to get response from LLM")
			continue
		}

		// ok, we'll have to go over all choices here as well
		whichWayToGo := make([]bool, len(tmp.GetCompletionResponse[0].Choices))
		success = false
		for summaryChoiceIdx, currentSummaryChoice := range tmp.GetCompletionResponse[0].Choices {
			tmp_currentSummary := strings.TrimSpace(currentSummaryChoice)
			parserErr := parser(tmp_currentSummary)
			if parserErr != nil {
				fmt.Printf("[%d] Error generating summary:\n```%s```... going to retry...!\n",
					aurora.BrightYellow(retryCounter),
					aurora.BrightRed(tmp_currentSummary))
				whichWayToGo[summaryChoiceIdx] = false
			} else {
				whichWayToGo[summaryChoiceIdx] = true
				currentSummary = tmp_currentSummary
				success = true
				// break ...?
			}
		}

		if !success {
			fmt.Printf("Failed to generate any summary for this step [%d/%d]\n",
				idx,
				len(snippets))
			goto retryGeneratingSummary
		}

		if time.Since(ts) > 5*time.Second {
			fmt.Printf("%s %02d/%02d, %s: %s\n",
				aurora.BrightGreen("Snippet"),
				idx+1,
				len(snippets),
				aurora.BrightYellow("question"),
				aurora.White(cutStringAt(question, 30)))
			fmt.Printf("[reduce] Current snippet: %s\n", aurora.White(cutStringAt(strings.TrimSpace(strings.ReplaceAll(snippet, "\n", " ")), 95)))
			fmt.Printf("[reduce] Current summary: %s\n", aurora.White(cutStringAt(currentSummary, 95)))
		}
	}

	if !success {
		fmt.Printf("[reduce] Failed to generate any summary for this text of [%d]\n",
			len(snippets))
		return ""
	}

	_ = storage.SaveTaskCacheResult("document-reduce-v1", fmt.Sprintf("%s\n%s",
		document, question), []byte(currentSummary))

	return currentSummary
}

func tokenizeDocument(document string, storage *cmds.Storage) ([]string, error) {
	// idea is to split document into batch of tokens of max length
	// max_snippet_size = (info.LLM.GetContextLength() * 2 / 3)
	// use info.LLM.Tokenize(document) to tokenize it
	tokenizerTs := time.Now()

	result, err := storage.GetTaskCachedResult("gpt2-tokenizer", document)
	if err != nil && result != nil {
		// ok, it's a hit...!
		parsedResult := make([]string, 0)
		err = json.Unmarshal(result, &parsedResult)
		if err == nil {
			return parsedResult, nil
		}
	}
	// if we're here - no valid cache records found..!

	// tokenization llm!
	tokenized, err := utils.TokenizeGPT2(document)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize document: %v", err)
	}

	snippetLength := 4096 * 1 / 7
	snippets := make([]string, 0)
	for i := 0; i < len(tokenized); i += snippetLength {
		end := i + snippetLength
		if end > len(tokenized) {
			end = len(tokenized)
		}

		snippets = append(snippets, utils.TokensToStringGPT2(tokenized[i:end]))
	}

	snippetsJSON, err := json.Marshal(snippets)
	if err == nil {
		_ = storage.SaveTaskCacheResult("gpt2-tokenizer", document, snippetsJSON)
	}

	zlog.Info().
		// Str("question", question).
		Dur("tokenizer", time.Since(tokenizerTs)).
		Int("snippet_count", len(snippets)).
		Msg("summarizing")
	return snippets, nil
}

func cutStringAt(content string, maxLen int) string {
	if len(content) < maxLen {
		return content
	}

	return content[:maxLen] + "..."
}
