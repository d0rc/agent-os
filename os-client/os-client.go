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

func (c *AgentOSClient) RunRequest(req *cmds.ClientRequest, timeout time.Duration) (*cmds.ServerResponse, error) {
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
