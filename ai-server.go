package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	process_embeddings "github.com/d0rc/agent-os/process-embeddings"
	"github.com/d0rc/agent-os/server"
	"github.com/d0rc/agent-os/utils"
	"io"
	"net/http"
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
	lg, logChan := utils.ConsoleInit("ai-srv", termUi)

	ctx, err := server.NewContext("config.yaml", lg, &server.Settings{
		TopInterval: time.Duration(*topInterval) * time.Millisecond,
		TermUI:      *termUi,
		LogChan:     logChan,
	})

	go ctx.Start(func(ctx *server.Context) {
		ctx.LaunchWorker("background{embeddings}", process_embeddings.BackgroundEmbeddingsWorker)
	})

	// start a http server on port 9000
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// read the request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			lg.Error().Err(err).Msg("failed to read request")
			return
		}
		defer r.Body.Close()

		// decode body JSON as ClientRequest
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
	var result *cmds.ServerResponse
	var err error
	if request.GetPageRequests != nil {
		// got some page requests...!
		result, err = cmds.ProcessPageRequests(request.GetPageRequests, ctx)
	}

	if request.GoogleSearchRequests != nil {
		result, err = cmds.ProcessGoogleSearches(request.GoogleSearchRequests, ctx)
	}

	if request.GetCompletionRequests != nil {
		result, err = cmds.ProcessGetCompletions(request.GetCompletionRequests, ctx, request.ProcessName, request.Priority)
	}

	if request.GetEmbeddingsRequests != nil {
		result, err = cmds.ProcessGetEmbeddings(request.GetEmbeddingsRequests, ctx, request.ProcessName, request.Priority)
	}

	if err != nil {
		return nil, err
	}

	result.CorrelationId = request.CorrelationId

	return result, nil
}
