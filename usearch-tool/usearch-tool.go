package main

import (
	"fmt"
	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	usearch "github.com/unum-cloud/usearch/golang"
	"math/rand"
	"os"
	"sort"
	"sync/atomic"
	"time"
)

func main() {
	coefficient := 128
	benchmarkConfigs := make([]benchmarkConfig, 0)
	for nThreads := 1; nThreads <= 10; nThreads++ {
		benchmarkConfigs = append(benchmarkConfigs, benchmarkConfig{
			vectorSize:             1536,
			vectorsCount:           1024 * coefficient,
			numberOfWritingThreads: nThreads,
			numberOfReadingThreads: nThreads,
		})
		benchmarkConfigs = append(benchmarkConfigs, benchmarkConfig{
			vectorSize:             4096,
			vectorsCount:           1024 * coefficient,
			numberOfWritingThreads: nThreads,
			numberOfReadingThreads: nThreads,
		})
	}

	type result struct {
		config benchmarkConfig
		result benchmarkResult
	}

	results := make([]result, 0, len(benchmarkConfigs))

	for _, config := range benchmarkConfigs {
		results = append(results, result{
			config: config,
			result: benchmark(config),
		})
	}

	// sort results, on highest speed
	sort.Slice(results, func(i, j int) bool {
		return results[i].result.writeSpeed > results[j].result.writeSpeed
	})

	tw := tablewriter.NewWriter(os.Stdout)
	tw.SetHeader([]string{"vector size", "vectors Count", "# w threads", "# r threads", "w duration", "w speed *", "r duration", "r speed"})
	for _, info := range results {
		tw.Append([]string{
			fmt.Sprintf("%d", info.config.vectorSize),
			fmt.Sprintf("%d", info.config.vectorsCount),
			fmt.Sprintf("%d", info.config.numberOfWritingThreads),
			fmt.Sprintf("%d", info.config.numberOfReadingThreads),
			fmt.Sprintf("%s", info.result.writeDuration),
			fmt.Sprintf("%5.2f", info.result.writeSpeed),
			fmt.Sprintf("%s", info.result.readDuration),
			fmt.Sprintf("%5.2f", info.result.readSpeed),
		})
	}

	tw.Render()
}

type benchmarkConfig struct {
	vectorSize             int
	vectorsCount           int
	numberOfWritingThreads int
	numberOfReadingThreads int
}
type benchmarkResult struct {
	writeSpeed    float32
	writeDuration time.Duration
	readSpeed     float32
	readDuration  time.Duration
}

func benchmark(config benchmarkConfig) benchmarkResult {
	conf := usearch.DefaultConfig(uint(config.vectorSize))
	index, err := usearch.NewIndex(conf)
	if err != nil {
		panic("Failed to create Index")
	}
	defer index.Destroy()

	vectorIdx := uint64(0)
	vectorsChan := make(chan []float32)
	done := make(chan struct{})
	fmt.Printf("\nStarting %d writer thread(s), to store vectors of size %d.\n",
		aurora.BrightGreen(config.numberOfWritingThreads),
		aurora.BrightGreen(config.vectorSize))
	for tIdx := 0; tIdx < config.numberOfWritingThreads; tIdx++ {
		go func(tIdx int) {
			for {
				select {
				case <-done:
					return
				case vector := <-vectorsChan:
					err := index.Add(usearch.Key(atomic.AddUint64(&vectorIdx, 1)), vector)
					if err != nil {
						panic(fmt.Sprintf("error writing vector %d: %s", vectorIdx, err.Error()))
					}
				}
			}
		}(tIdx)
	}

	// Add to Index
	tsw := time.Now()
	lastStatsPrintedAt := time.Now()
	err = index.Reserve(uint(config.vectorsCount))
	if err != nil {
		panic(fmt.Sprintf("error reserving index: %s", err.Error()))
	}
	nextLine := "\n"
	printNewLine := false
	for idx := 0; idx < config.vectorsCount; idx++ {
		vector := randomVector(config.vectorSize)
		vectorsChan <- vector

		if time.Since(lastStatsPrintedAt) > 1*time.Second {
			fmt.Printf("[%4ds] Created %5d vectors, added %5d vectors, at the speed of %5.5f (vectors/second).%s",
				aurora.BrightCyan(int(time.Since(tsw).Seconds())),
				aurora.BrightGreen(atomic.LoadUint64(&vectorIdx)),
				aurora.Green(idx),
				aurora.BrightYellow(float32(idx)/float32(time.Since(tsw).Seconds())),
				nextLine)
			lastStatsPrintedAt = time.Now()
			if nextLine == "\r" {
				printNewLine = true
			}
		}
		if time.Since(tsw) > 10*time.Second {
			nextLine = "\r"
		}
	}
	for tIdx := 0; tIdx < config.numberOfWritingThreads; tIdx++ {
		done <- struct{}{}
	}
	if printNewLine {
		fmt.Printf("\n")
	}
	fmt.Printf("Done saving vectors in %v.\n", aurora.BrightCyan(time.Since(tsw)))
	fmt.Printf("Adding speed: %5.5f (vectors/second)\n", float32(config.vectorsCount)/float32(time.Since(tsw).Seconds()))

	// now, lets see how fast we can search these vectors
	fmt.Printf("\nStarting %d read thread(s), to search vectors of size %d.\n",
		aurora.BrightGreen(config.numberOfWritingThreads),
		aurora.BrightGreen(config.vectorSize))
	for tIdx := 0; tIdx < config.numberOfReadingThreads; tIdx++ {
		go func(tIdx int) {
			for {
				select {
				case <-done:
					return
				case vector := <-vectorsChan:
					_, _, err := index.Search(vector, 10)
					if err != nil {
						panic(fmt.Sprintf("error reading vector %d: %s", vectorIdx, err.Error()))
					}
				}
			}
		}(tIdx)
	}
	tsr := time.Now()
	nextLine = "\n"
	printNewLine = false
	for idx := 0; idx < config.vectorsCount; idx++ {
		vector := randomVector(config.vectorSize)
		vectorsChan <- vector

		if time.Since(lastStatsPrintedAt) > 1*time.Second {
			fmt.Printf("[%4ds] Searching %5d of %5d vectors, at the speed of %5.5f (queries/second).%s",
				aurora.BrightCyan(int(time.Since(tsr).Seconds())),
				aurora.Green(idx),
				aurora.BrightGreen(atomic.LoadUint64(&vectorIdx)),
				aurora.BrightYellow(float32(idx)/float32(time.Since(tsr).Seconds())),
				nextLine)
			lastStatsPrintedAt = time.Now()
		}
		if time.Since(tsr) > 10*time.Second {
			nextLine = "\r"
		}
		if nextLine == "\r" {
			printNewLine = true
		}
	}
	for tIdx := 0; tIdx < config.numberOfReadingThreads; tIdx++ {
		done <- struct{}{}
	}
	if printNewLine {
		fmt.Printf("\n")
	}
	fmt.Printf("Done searching vectors in %v.\n", aurora.BrightCyan(time.Since(tsr)))
	fmt.Printf("Search speed: %5.5f (queries/second)\n", float32(config.vectorsCount)/float32(time.Since(tsr).Seconds()))

	return benchmarkResult{
		writeSpeed:    float32(config.vectorsCount) / float32(time.Since(tsw).Seconds()),
		writeDuration: time.Since(tsw),
		readSpeed:     float32(config.vectorsCount) / float32(time.Since(tsr).Seconds()),
		readDuration:  time.Since(tsr),
	}
}

func randomVector(n int) []float32 {
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		vec[i] = rand.Float32()
	}
	return vec
}
