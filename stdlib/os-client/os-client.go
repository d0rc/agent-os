package os_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"
)

type AgentOSClient struct {
	Url    string
	client http.Client
}

func NewAgentOSClient(url string) *AgentOSClient {
	tr := &http.Transport{
		MaxIdleConns:          10,
		IdleConnTimeout:       15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		DisableKeepAlives:     false,
	}
	return &AgentOSClient{
		Url: url,
		client: http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
	}
}

func (c *AgentOSClient) RunRequests(reqs []*cmds.ClientRequest, timeout time.Duration) ([]*cmds.ServerResponse, error) {
	responses := make([]chan *cmds.ServerResponse, len(reqs))
	for idx, req := range reqs {
		responses[idx] = make(chan *cmds.ServerResponse)
		go func(req *cmds.ClientRequest, ch chan *cmds.ServerResponse) {
			resp := c.RunRequest(req, timeout, REP_Default)
			ch <- resp
		}(req, responses[idx])
	}

	finalResponses := make([]*cmds.ServerResponse, 0)
	for _, ch := range responses {
		finalResponses = append(finalResponses, <-ch)
	}

	return finalResponses, nil
}

type RequestExecutionPool int

const (
	REP_Default RequestExecutionPool = iota
	REP_IO
)

var maxParallelRequestsChannel = make(chan struct{}, 256)

func (c *AgentOSClient) RunRequest(req *cmds.ClientRequest, timeout time.Duration, executionPool RequestExecutionPool) *cmds.ServerResponse {
	//timeout = 60 * time.Second
	if req.SpecialCaseResponse != "" || isRequestEmpty(req) {
		return &cmds.ServerResponse{
			SpecialCaseResponse: req.SpecialCaseResponse,
			CorrelationId:       req.CorrelationId,
		}
	}

	if executionPool == REP_Default {
		maxParallelRequestsChannel <- struct{}{}
		defer func() {
			<-maxParallelRequestsChannel
		}()
	}
	req.Trx = uuid.New().String()
retry:

	reqBytes, err := json.Marshal(req)
	if err != nil {
		zlog.Fatal().Msgf("error marshalling request: %v", err)
	}

	resp, err := c.client.Post(c.Url, "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		fmt.Printf("%s running OS request, going to re-try: %v\n",
			aurora.BrightRed("error"),
			aurora.BrightGreen(err))
		time.Sleep(300 * time.Millisecond)
		goto retry
	}

	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("%s read OS response, going to re-try: %v\n",
			aurora.BrightRed("error"),
			aurora.BrightGreen(err))
		time.Sleep(300 * time.Millisecond)
		goto retry
	}

	var serverResponse cmds.ServerResponse
	err = json.Unmarshal(respBytes, &serverResponse)
	if err != nil {
		fmt.Printf("%s processing OS response, going to re-try: %v\n",
			aurora.BrightRed("error"),
			aurora.BrightGreen(err))
		time.Sleep(300 * time.Millisecond)
		goto retry
	}

	return &serverResponse
}

func isRequestEmpty(req *cmds.ClientRequest) bool {
	isEmpty := true
	isEmpty = isEmpty && (req.GetEmbeddingsRequests == nil || len(req.GetEmbeddingsRequests) == 0)
	isEmpty = isEmpty && (req.GetCompletionRequests == nil || len(req.GetCompletionRequests) == 0)
	isEmpty = isEmpty && (req.GetPageRequests == nil || len(req.GetPageRequests) == 0)
	isEmpty = isEmpty && (req.GetCacheRecords == nil || len(req.GetCacheRecords) == 0)
	isEmpty = isEmpty && (req.SetCacheRecords == nil || len(req.SetCacheRecords) == 0)
	isEmpty = isEmpty && (req.GoogleSearchRequests == nil || len(req.GoogleSearchRequests) == 0)

	isEmpty = isEmpty && (req.WriteMessagesTrace == nil || len(req.WriteMessagesTrace) == 0)

	return isEmpty
}
