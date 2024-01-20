package tools

import (
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"strings"
)

func FlattenChoices(response []*cmds.GetCompletionResponse) []string {
	result := make([]string, 0)
	for _, choice := range response {
		for _, c := range choice.Choices {
			result = append(result, c)
		}
	}

	return result
}

func Replicate[T any](request T, results int) []T {
	result := make([]T, results)
	for i := 0; i < results; i++ {
		result[i] = request
	}

	return result
}

func DropDuplicates(list []string) []string {
	var result = make([]string, 0)
	seen := make(map[string]struct{})

	for _, item := range list {
		item = strings.TrimSpace(item)
		if _, ok := seen[item]; !ok {
			result = append(result, item)
			seen[item] = struct{}{}
		}
	}

	return result
}

func CodeBlock(s string) string {
	return fmt.Sprintf("```\n%s\n```", s)
}
