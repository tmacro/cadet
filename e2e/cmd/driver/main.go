package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gorilla/mux"
	"github.com/tmacro/cadet/pkg/httpserver"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/supervise"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var CLI struct {
	LogLevel  string `help:"Set the log level." enum:"debug,info,warn,error" default:"debug"`
	LogFormat string `enum:"json,text" default:"text" help:"Set the log format. (json, text)"`

	Addr string `help:"address to bind" default:"127.0.0.1:8080"`
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
	r.HandleFunc("/request", handleRequest).Methods("POST")
	r.HandleFunc("/_/healthcheck", handleHealthcheck).Methods("GET")
	return r
}

func handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	log.Debug("Handling healthcheck request")
	w.WriteHeader(http.StatusOK)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req Request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	log.Debug("Proxying request")

	resp := DoRequest(req)
	err = json.NewEncoder(w).Encode(&resp)
	if err != nil {
		log.Error(err)
	}
}

type Request struct {
	URL    string `json:"url"`
	Host   string `json:"host"`
	Method string `json:"method"`
}

type Response struct {
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"`
	Err    string `json:"error,omitempty"`
}

func DoRequest(r Request) Response {
	status, body, err := GetURL(r.URL, r.Host)
	log.Debugf("proxied req GET %s - %d - %s", r.Host, status, string(body))
	if err != nil {
		return Response{Err: err.Error()}
	}

	return Response{Status: status, Body: string(body)}
}

func GetURL(rawUrl string, host string) (int, []byte, error) {
	transport := &http.Transport{}
	if host != "" {
		u, err := url.Parse(rawUrl)
		if err != nil {
			return 0, nil, err
		}

		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, u.Host)
		}

		transport.TLSClientConfig = &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
	}

	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", rawUrl, nil)
	if err != nil {
		return 0, nil, err
	}

	if host != "" {
		req.Host = host
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}
