package utils

import (
	"flag"
	"fmt"
	"github.com/logrusorgru/aurora"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"os"
	"runtime"
	"strings"
	"syscall"
)

type ChannelWriter struct {
	Channel chan []byte
}

func (cw ChannelWriter) Write(p []byte) (n int, err error) {
	cw.Channel <- p
	return len(p), nil
}

var OutputChannel = make(chan []byte, 1024)

func ConsoleInit(name string, termUi *bool) (zerolog.Logger, chan []byte) {
	flag.Parse()
	logsInit(termUi)

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

func logsInit(termUi *bool) {
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
