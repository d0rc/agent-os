package tools

import (
	"fmt"
	"strings"
)

func ParseJSON(sourceData string, parser func(string) error) error {
	// starting with some symbol, it's JSON here, and it ends with some symbol
	// first symbol of JSON can be: '"', "{", "["
	sourceData = cleanJSONString(sourceData)
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

	return err
}

func cleanJSONString(input string) string {
	// Replace <0x0A> with \n or remove it if newlines are not required
	return strings.Replace(input, "<0x0A>", "\n", -1)
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
