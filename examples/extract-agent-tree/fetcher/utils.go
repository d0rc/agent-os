package fetcher

func getMapKeys(messages map[string]struct{}) []string {
	result := make([]string, 0, len(messages))
	for s, _ := range messages {
		result = append(result, s)
	}

	return result
}

func pMap[T any](f func([]string, int) []T, data [][]string, maxThreads int) []T {
	maxThreadsChan := make(chan struct{}, maxThreads)
	resultsChannels := make([]chan []T, len(data))
	for i := 0; i < len(data); i++ {
		resultsChannels[i] = make(chan []T, 1)
	}
	for idx, chunk := range data {
		maxThreadsChan <- struct{}{}
		go func(idx int, chunk []string) {
			defer func() { <-maxThreadsChan }()
			tmp := f(chunk, idx)
			resultsChannels[idx] <- tmp
		}(idx, chunk)
	}

	results := make([]T, 0)
	for idx := 0; idx < len(data); idx++ {
		results = append(results, <-resultsChannels[idx]...)
	}

	return results
}
