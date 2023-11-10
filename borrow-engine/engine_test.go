package borrow_engine

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

func TestComputeRoutingWorksTest(t *testing.T) {
	// create engine
	engine := NewInferenceEngine(ComputeFunction{
		JT_Completion: func(node *InferenceNode, jobs []*ComputeJob) []*ComputeJob {
			//fmt.Printf("Running batch of %d jobs on node %s\n", len(jobs), node.EndpointUrl)
			time.Sleep(time.Duration(1+rand.Intn(1000)) * time.Millisecond)
			return jobs
		},
		JT_Embeddings: func(node *InferenceNode, jobs []*ComputeJob) []*ComputeJob {
			//fmt.Printf("Running batch of %d jobs on node %s\n", len(jobs), node.EndpointUrl)
			time.Sleep(time.Duration(1+rand.Intn(1000)) * time.Millisecond)
			return jobs
		},
	})
	go engine.Run()

	// add ten random nodes with
	// max requests 1-2
	// max batch size 70-100
	// job types: JT_Completion, JT_Embeddings
	for i := 0; i < 5; i++ {
		node := &InferenceNode{
			EndpointUrl:  fmt.Sprintf("http://127.0.0.1:800%d/v1/completions", i),
			MaxRequests:  1 + rand.Intn(2),
			MaxBatchSize: 128,
			JobTypes:     []JobType{JT_Completion, JT_Embeddings},
		}
		engine.AddNode(node)
	}

	processes := []string{
		"agent-test",
		"background[embeddings]",
		"agent-user-chat",
		"agent-project-manager",
		"agent-c++-developer",
		"agent-python-developer",
		"agent-frontend-developer",
		"agent-html5-developer",
		"agent-general-research",
		"background[default-mode-network]",
	}

	for {
		// add 1000 random jobs with random priority
		// and random job type
		for i := 0; i < 100_000; i++ {
			job := &ComputeJob{
				JobId:    fmt.Sprintf("job-%d", i),
				JobType:  JobType(rand.Intn(2)),
				Priority: RequestPriority(rand.Intn(4)),
				Process:  processes[(rand.Int()%len(processes)+rand.Int()%len(processes)+rand.Int()%len(processes))/3],
			}
			engine.AddJob(job)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
