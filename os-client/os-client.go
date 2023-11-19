package os_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"io"
	"net/http"
	"time"
)

type AgentOSClient struct {
	Url string
}

func NewAgentOSClient(url string) *AgentOSClient {
	return &AgentOSClient{Url: url}
}

func (c *AgentOSClient) RunRequests(reqs []*cmds.ClientRequest, timeout time.Duration) ([]*cmds.ServerResponse, error) {
	type enumeratedResponse struct {
		Idx  int
		Resp *cmds.ServerResponse
	}
	responses := make([]chan *enumeratedResponse, len(reqs))
	for idx, req := range reqs {
		responses[idx] = make(chan *enumeratedResponse)
		go func(req *cmds.ClientRequest, ch chan *enumeratedResponse, idx int) {
			resp, err := c.RunRequest(req, timeout)
			if err != nil {
				fmt.Printf("error running request: %v\n", err)
			}

			ch <- &enumeratedResponse{
				Idx:  idx,
				Resp: resp,
			}
		}(req, responses[idx], idx)
	}

	finalResponses := make([]*cmds.ServerResponse, len(reqs))
	for _, ch := range responses {
		resp := <-ch
		finalResponses[resp.Idx] = resp.Resp
	}

	return finalResponses, nil
}

var maxParallelRequestsChannel = make(chan struct{}, 100)

func (c *AgentOSClient) RunRequest(req *cmds.ClientRequest, timeout time.Duration) (*cmds.ServerResponse, error) {
	maxParallelRequestsChannel <- struct{}{}
	defer func() {
		<-maxParallelRequestsChannel
	}()
	client := http.Client{Timeout: timeout}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	resp, err := client.Post(c.Url, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var serverResponse cmds.ServerResponse
	err = json.Unmarshal(respBytes, &serverResponse)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &serverResponse, nil
}
