package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/metrics"
	process_embeddings "github.com/d0rc/agent-os/process-embeddings"
	"github.com/d0rc/agent-os/server"
	"github.com/d0rc/agent-os/utils"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"
)

// the aim of the project is to provide agents with a way to
// manage internal prompting data, handle inference, and facilitate
// external calls, such as web browsing or calling scripts in different environments
// this code was created with the help of LLMs and Tabby server

var port = flag.Int("port", 9000, "port to listen on")
var host = flag.String("host", "0.0.0.0", "host to listen at")
var topInterval = flag.Int("top-interval", 1000, "interval to update `top` (ms)")
var termUi = flag.Bool("term-ui", true, "enable term ui")

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	lg, logChan := utils.ConsoleInit("ai-srv", termUi)

	ctx, err := server.NewContext("config.yaml", lg, &server.Settings{
		TopInterval: time.Duration(*topInterval) * time.Millisecond,
		TermUI:      *termUi,
		LogChan:     logChan,
	})

	if err != nil {
		log.Fatalf("failed to create context: %v", err)
	}

	go ctx.Start(func(ctx *server.Context) {
		ctx.LaunchWorker("background{embeddings}", process_embeddings.BackgroundEmbeddingsWorker)
	})

	// start a http server on port 9000
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		metrics.Tick("http.requests", 1)
		// read the request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			lg.Error().Err(err).Msg("failed to read request")
			return
		}
		_ = r.Body.Close()

		clientRequest := &cmds.ClientRequest{}
		err = json.Unmarshal(body, clientRequest)
		if err != nil {
			lg.Error().Err(err).Msg("error parsing client request")
			return
		}

		resp, err := processRequest(clientRequest, ctx)

		respBytes, err := json.Marshal(resp)
		if err != nil {
			lg.Error().Err(err).Msg("error serializing server response")
			return
		}

		_, err = w.Write(respBytes)
		if err != nil {
			lg.Error().Err(err).Msg("error sending server response")
		}
	})

	workingHost := fmt.Sprintf("%s:%d", *host, *port)
	lg.Info().Msgf("starting on: %s", workingHost)
	err = http.ListenAndServe(workingHost, nil)
	if err != nil {
		lg.Fatal().Err(err).Msg("error starting server")
	}
}

func processRequest(request *cmds.ClientRequest, ctx *server.Context) (*cmds.ServerResponse, error) {
	var result *cmds.ServerResponse = &cmds.ServerResponse{}
	var err error

	if request.GetPageRequests != nil && len(request.GetPageRequests) > 0 {
		// got some page requests...!
		ctx.ComputeRouter.AccountProcessRequest(request.ProcessName)
		result, err = cmds.ProcessPageRequests(request.GetPageRequests, ctx)
	}

	if request.GoogleSearchRequests != nil && len(request.GoogleSearchRequests) > 0 {
		ctx.ComputeRouter.AccountProcessRequest(request.ProcessName)
		result, err = cmds.ProcessGoogleSearches(request.GoogleSearchRequests, ctx)
	}

	if request.GetCompletionRequests != nil && len(request.GetCompletionRequests) > 0 {
		ctx.ComputeRouter.AccountProcessRequest(request.ProcessName)
		result, err = cmds.ProcessGetCompletions(request.GetCompletionRequests, ctx, request.ProcessName, request.Priority)
	}

	if request.GetEmbeddingsRequests != nil && len(request.GetEmbeddingsRequests) > 0 {
		ctx.ComputeRouter.AccountProcessRequest(request.ProcessName)
		result, err = cmds.ProcessGetEmbeddings(request.GetEmbeddingsRequests, ctx, request.ProcessName, request.Priority)
	}

	if request.GetCacheRecords != nil && len(request.GetCacheRecords) > 0 {
		// ctx.ComputeRouter.AccountProcessRequest(request.ProcessName)
		result, err = cmds.ProcessGetCacheRecords(request.GetCacheRecords, ctx, request.ProcessName)
	}

	if request.SetCacheRecords != nil && len(request.SetCacheRecords) > 0 {
		// ctx.ComputeRouter.AccountProcessRequest(request.ProcessName)
		result, err = cmds.ProcessSetCacheRecords(request.SetCacheRecords, ctx, request.ProcessName)
	}

	if request.WriteMessagesTrace != nil && len(request.WriteMessagesTrace) > 0 {
		result, err = cmds.ProcessWriteMessagesTrace(request.ProcessName, request.WriteMessagesTrace, ctx, request.ProcessName)
	}

	if request.UIRequest != nil {
		result, err = cmds.ProcessUIRequest(request.UIRequest, ctx)
	}

	if err != nil {
		return nil, err
	}

	result.CorrelationId = request.CorrelationId
	result.SpecialCaseResponse = request.SpecialCaseResponse

	return result, nil
}
