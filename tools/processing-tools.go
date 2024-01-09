package tools

import "github.com/d0rc/agent-os/cmds"

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
