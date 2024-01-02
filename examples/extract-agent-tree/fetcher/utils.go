package fetcher

func getMapKeys(messages map[string]struct{}) []string {
	result := make([]string, 0, len(messages))
	for s, _ := range messages {
		result = append(result, s)
	}

	return result
}
