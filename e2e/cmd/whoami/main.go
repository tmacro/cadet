package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/gorilla/mux"
	"github.com/tmacro/cadet/pkg/httpserver"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/supervise"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var CLI struct {
	LogLevel  string `help:"Set the log level." enum:"debug,info,warn,error" default:"info"`
	LogFormat string `enum:"json,text" default:"text" help:"Set the log format. (json, text)"`

	Addr string `help:"address to bind" default:"127.0.0.1:8080"`
	Name string `help:"Name to respond with." default:"whoami"`
}

func main() {
	cmd := kong.Parse(&CLI,
		kong.Name("whoami"),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	var logLevel zapcore.Level
	switch CLI.LogLevel {
	case "debug":
		logLevel = zapcore.DebugLevel
	case "info":
		logLevel = zapcore.InfoLevel
	case "warn":
		logLevel = zapcore.WarnLevel
	case "error":
		logLevel = zapcore.ErrorLevel
	default:
		panic("invalid log level: " + CLI.LogLevel)
	}

	encoderCfg := zap.NewDevelopmentEncoderConfig()
	// encoderCfg.StacktraceKey = ""
	// encoderCfg.FunctionKey = ""
	// encoderCfg.CallerKey = ""

	logger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		logLevel,
	))

	defer logger.Sync()

	log.SetLogger(logger.Sugar())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	services := []supervise.Service{
		httpserver.Server(CLI.Addr, newRouter(), httpserver.RequestLogger),
	}

	err := supervise.Serve(ctx, services...)
	cmd.FatalIfErrorf(err)
}

func newRouter() http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/", handleName(CLI.Name)).Methods("GET")
	r.HandleFunc("/ip", handleGetIP).Methods("GET")
	r.HandleFunc("/_/healthcheck", handleHealthcheck).Methods("GET")
	return r
}

func handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	log.Debug("Handling healthcheck request")
	w.WriteHeader(http.StatusOK)
}

func handleName(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(name))
	}
}

func handleGetIP(w http.ResponseWriter, r *http.Request) {
	ip := getIP()

	w.Write([]byte(ip))
}

func getIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Print(fmt.Errorf("localAddresses: %+v\n", err.Error()))
		return ""
	}

	for _, i := range ifaces {
		if i.Name == "eth0" {
			addrs, err := i.Addrs()
			if err != nil {
				fmt.Print(fmt.Errorf("localAddresses: %+v\n", err.Error()))
				return ""
			}

			for _, a := range addrs {
				parts := strings.Split(a.String(), "/")
				return parts[0]
			}
		}
	}

	return ""
}
