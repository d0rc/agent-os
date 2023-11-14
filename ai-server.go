package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
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
	lg, logChan := consoleInit("ai-srv")

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

type ChannelWriter struct {
	Channel chan []byte
}

func (cw ChannelWriter) Write(p []byte) (n int, err error) {
	cw.Channel <- p
	return len(p), nil
}

var OutputChannel = make(chan []byte, 1024)

func consoleInit(name string) (zerolog.Logger, chan []byte) {
	flag.Parse()
	if name != "" {
		if *termUi {
			return zerolog.New(ChannelWriter{Channel: OutputChannel}).With().Str("app", name).Logger(), OutputChannel
		} else {
			return zlog.With().Str("app", name).Logger(), OutputChannel
		}
	} else {
		if *termUi {
			return zerolog.New(ChannelWriter{Channel: OutputChannel}), OutputChannel
		} else {
			return zlog.Logger, OutputChannel
		}
	}
}

func init() {
	if *termUi {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		zlog.Logger = zlog.
			Output(ChannelWriter{Channel: OutputChannel}).
			Hook(LineInfoHook{})
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		zlog.Logger = zlog.
			Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "02/01 15:04:05"}).
			Hook(LineInfoHook{})
	}
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
