package tools

import (
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/server"
	zlog "github.com/rs/zerolog/log"
	"strings"
)

func ParseJSON(sourceData string, parser func(string) error) error {
	// starting with some symbol, it's JSON here, and it ends with some symbol
	// first symbol of JSON can be: '"', "{", "["
	sourceData = strings.TrimSpace(strings.ReplaceAll(sourceData, "\\|", "|"))

	if len(sourceData) == 1 {
		return fmt.Errorf("json is too short...!")
	}

	jsonStartingSymbols := []string{"{", "[", "\"", "1", "2", "3", "4", "5", "6", "7", "8", "9", ".", "0", "t", "f"}

	var err error
	for _, symbol := range jsonStartingSymbols {
		if bracketIndex := strings.Index(sourceData, symbol); bracketIndex != -1 {
			err = actualParse(strings.TrimSpace(sourceData[bracketIndex:]), parser)
			if err == nil {
				return nil
			}
		}
	}

	if err == nil {
		return err
	}

	/*
		for minPosition := 1; minPosition < len(sourceData)-1; minPosition++ {
			err = actualParse(sourceData[minPosition-1:], parser)
			if err == nil {
				return nil
			}
		}*/

	return err
}

func actualParse(newSourceData string, parser func(string) error) error {
	// so JSON starts at position minPosition, let create a new string starting from minPosition
	//newSourceData = strings.TrimSpace(newSourceData)

	// let generate all strings, starting with full newSourceData, then newSourceData with last symbol removed
	// then next symbol from the tail removed, and so on up to only 1 symbol left
	for i := 0; i < len(newSourceData); i++ {
		tmpSourceData := newSourceData[:len(newSourceData)-i]

		err := parser(tmpSourceData)
		if err == nil {
			return nil
		}
	}

	// in case of GPT 3.5 one may try to add "}" symbol to the end of the string and try to parse it again
	newSourceData = newSourceData + "}"
	err := parser(newSourceData)
	if err == nil {
		return nil
	}

	return fmt.Errorf("failed to parse json: %v", err)
}

func LLMJSONParser(text string, ctx *server.Context, model string, parser func(string) error) error {
	prompt := fmt.Sprintf(`### Instruction
Answer the question: Why this JSON is broken?

%s

Take  a deep breath, think step by step. Check all brackets in place, no extra-brackets, wrong escape symbols, extra formatting or punctuation, like "...." or other symbols coming from schema descriptions. Best way to ensure correctness is to try to rewrite JSON, while skipping formatting symbols, i.e. output minified version.
### Assistant:`,
		text)

	// no, since we have prompt we can attempt to execute one more time
	// and attempt parsing all returned variants, applying ParseJSON function
	res, err := cmds.ProcessGetCompletions([]cmds.GetCompletionRequest{
		{
			Model:       model,
			RawPrompt:   prompt,
			Temperature: 0.5,
			StopTokens:  []string{"###"},
			MinResults:  100,
		},
	}, ctx, "json-fixer", borrow_engine.PRIO_User)
	if err != nil {
		zlog.Error().Err(err).Msgf("failed to get completions in json-fixer")
		return err
	}

	for _, choice := range res.GetCompletionResponse[0].Choices {
		err = ParseJSON(choice, parser)
		if err == nil {
			return nil
		}
	}

	return err
}

func TwoStepJSONParser(text string, ctx *server.Context, model string, parser func(string) error) error {
	err := ParseJSON(text, parser)
	if err == nil {
		return nil
	}

	return LLMJSONParser(text, ctx, model, parser)
}
