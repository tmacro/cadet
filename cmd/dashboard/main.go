package main

import (
	"context"
	"io/fs"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tmacro/cadet/internal/webui"
	"github.com/tmacro/cadet/pkg/httpserver"

	"github.com/alecthomas/kong"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/tmacro/cadet/pkg/config"
	"github.com/tmacro/cadet/pkg/consul"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
	"github.com/tmacro/cadet/pkg/supervise"
)

var CLI struct {
	LogLevel  string `help:"Set the log level." enum:"debug,info,warn,error" default:"info"`
	LogFormat string `enum:"json,text" default:"text" help:"Set the log format. (json, text)"`

	Addr      string `help:"address to bind" default:"127.0.0.1:8080"`
	Templates string `help:"directory containing ui templates"`
	Static    string `help:"directory containing static assets"`

	URL                string `help:"Consul endpoint to connect to." default:"http://localhost:8500"`
	GlobalConfigPrefix string `help:"Consul global config prefix." default:"cadet"`
	ServiceTag         string `help:"Service tag to watch for." default:"cadet"`
	NoPublishTag       string `help:"Service tag to not publish." default:"cadet-no-publish"`

	ZoneKey     string `help:"Consul metadata key for zone." default:"cadet-zone"`
	ACLKey      string `help:"Consul metadata key for ACL." default:"cadet-acl"`
	NameKey     string `help:"Consul metadata key for name." default:"cadet-name"`
	TargetKey   string `help:"Consul metadata key for target." default:"cadet-target"`
	UseProxyKey string `help:"Consul metadata key for Disabling proxying." default:"cadet-proxy"`
}

func main() {
	cmd := kong.Parse(&CLI,
		kong.Name("cadet-dashboard"),
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

	log.Infof("Starting cadet dashboard")

	endpoint, err := url.Parse(CLI.URL)
	cmd.FatalIfErrorf(err, "invalid consul endpoint")

	catalog, kv, err := consul.CreateClient(endpoint.Scheme, endpoint.Host, "")
	cmd.FatalIfErrorf(err, "failed to create consul client: %v")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	extractCfg := service.ExtractConfig{
		ServiceTag:   CLI.ServiceTag,
		NoPublishTag: CLI.NoPublishTag,
		ZoneKey:      CLI.ZoneKey,
		ACLKey:       CLI.ACLKey,
		NameKey:      CLI.NameKey,
		UseProxyKey:  CLI.UseProxyKey,
	}

	watcher := config.NewWatcher(onChange, onError)

	templates := webui.EmbeddedTemplates
	if CLI.Templates != "" {
		templates = webui.TemplateCacheFromPath(CLI.Templates, webui.NoCache)
	}

	var staticAssets fs.FS
	if CLI.Static != "" {
		staticAssets = webui.AssetsFromPath(CLI.Static)
	} else {
		staticAssets = webui.StaticAssets
	}

	services := []supervise.Service{
		supervise.ServiceFunc(func(ctx context.Context) error {
			return watcher.Run(ctx, catalog, kv, CLI.GlobalConfigPrefix, extractCfg)
		}),
		httpserver.Server(CLI.Addr, webui.NewHandler(watcher, templates, staticAssets), httpserver.RequestLogger),
	}

	err = supervise.Serve(ctx, services...)
	cmd.FatalIfErrorf(err)
}

func onChange(rc config.RemoteConfig) bool {
	if rc.Ready() {
		log.Debugf("config updated: %d services", len(rc.Services()))
	}
	return true
}

func onError(err error) (time.Duration, bool) {
	log.Error(err)
	return 0, false
}
