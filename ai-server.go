package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	process_embeddings "github.com/d0rc/agent-os/process-embeddings"
	"github.com/d0rc/agent-os/server"
	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
)

// the aim of the project is to provide agents with a way to
// manage internal prompting data, handle inference, and facilitate
// external calls, such as web browsing or calling scripts in different environments
// this code was created with the help of LLMs and Tabby server

var port = flag.Int("port", 9000, "port to listen on")
var host = flag.String("host", "0.0.0.0", "host to listen at")

func main() {
	lg := consoleInit("ai-srv")

	ctx, err := server.NewContext("config.yaml", lg)

	engines.StartInferenceEngines()
	go cmds.ProcessJobsQueue()
	go ctx.Start(process_embeddings.BackgroundEmbeddingsWorker)

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

func consoleInit(name string) zerolog.Logger {
	flag.Parse()

	if name != "" {
		return zlog.With().Str("app", name).Logger()
	} else {
		return zlog.Logger
	}
}

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zlog.Logger = zlog.
		Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "02/01 15:04:05"}).
		Hook(LineInfoHook{})
	setupForHighLoad()
}

func setupForHighLoad() {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		fmt.Printf("%s Failed to get rlimit: %s\n",
			aurora.Red("ERR"), err.Error())
	} else {
		var changed = false
		if rLimit.Cur < 256000 {
			rLimit.Cur = 256000
			changed = true
		}
		if rLimit.Max < 256000 {
			rLimit.Max = 256000
			changed = true
		}
		if !changed {
			fmt.Printf("%s current=%v, max=%v is enough, no changes needed\n",
				aurora.Green("rlimit"), rLimit.Cur, rLimit.Max)
			return
		}

		err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		if err != nil {
			fmt.Printf("%s Failed to set rlimit: %s\n",
				aurora.Red("ERR"), err.Error())
		} else {
			err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
			if err != nil {
				fmt.Printf("%s Failed to check rlimit: %s\n",
					aurora.Red("ERR"), err.Error())
			} else {
				fmt.Printf("%s changed to current=%v, max=%v\n",
					aurora.Green("rlimit"), rLimit.Cur, rLimit.Max)
			}
		}
	}
}

type LineInfoHook struct{}

func (h LineInfoHook) Run(e *zerolog.Event, l zerolog.Level, msg string) {
	if l >= zerolog.InfoLevel {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			file = file[strings.Index(file, "agent-os/")+8:]
			e.Str("line", fmt.Sprintf("%s:%d", file, line))
		}
	}
}

func processRequest(request *cmds.ClientRequest, ctx *server.Context) (*cmds.ServerResponse, error) {
	if request.GetPageRequests != nil {
		// got some page requests...!
		return cmds.ProcessPageRequests(request.GetPageRequests, ctx)
	}

	if request.GoogleSearchRequests != nil {
		return cmds.ProcessGoogleSearches(request.GoogleSearchRequests, ctx)
	}

	if request.GetCompletionRequests != nil {
		return cmds.ProcessGetCompletions(request.GetCompletionRequests, ctx)
	}

	return nil, nil
}
