package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/tmacro/cadet/pkg/config"

	"github.com/alecthomas/kong"
	//	"github.com/hashicorp/consul/api"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/tmacro/cadet/pkg/consul"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
)

var CLI struct {
	LogLevel           string `help:"Set the log level." enum:"debug,info,warn,error" default:"info"`
	LogFormat          string `enum:"json,text" default:"text" help:"Set the log format. (json, text)"`
	URL                string `help:"Consul endpoint to connect to." default:"http://localhost:8500"`
	GlobalConfigPrefix string `help:"Consul global config prefix." default:"cadet"`
	ServiceTag         string `help:"Service tag to watch for." default:"cadet"`
	NoPublishTag       string `help:"Service tag to not publish." default:"cadet-no-publish"`

	ZoneKey     string `help:"Consul metadata key for zone." default:"cadet-zone"`
	ACLKey      string `help:"Consul metadata key for ACL." default:"cadet-acl"`
	NameKey     string `help:"Consul metadata key for name." default:"cadet-name"`
	TargetKey   string `help:"Consul metadata key for target." default:"cadet-target"`
	UseProxyKey string `help:"Consul metadata key for Disabling proxying." default:"cadet-use-proxy"`
}

func main() {
	kong.Parse(&CLI,
		kong.Name("watch-services"),
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

	logger := zap.New(zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderCfg),
		zapcore.Lock(os.Stdout),
		logLevel,
	))

	defer logger.Sync()

	log.SetLogger(logger.Sugar())

	log.Infof("Starting watch-services")

	endpoint, err := url.Parse(CLI.URL)
	if err != nil {
		log.Fatal("invalid consul endpoint: %v", err)
	}

	client, kv, err := consul.CreateClient(endpoint.Scheme, endpoint.Host, "")
	if err != nil {
		log.Fatal("failed to create consul client: %v", err)
	}

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

	err = watcher.Run(ctx, client, kv, CLI.GlobalConfigPrefix, extractCfg)
	if err != nil {
		log.Fatal(err)
	}
}

func onChange(rc config.RemoteConfig) bool {
	for _, svc := range rc.Services() {
		fmt.Println("Service:", svc.ID)
		fmt.Printf("\tType: %s\n", svc.Type)
		fmt.Printf("\tName: %s\n", svc.Name)
		fmt.Print("\tEndpoints: [ ")
		for _, ep := range svc.Endpoints {
			fmt.Printf("%s ", ep)
		}
		fmt.Println("]")

		fmt.Print("\tZones: ")
		if len(svc.Zones) > 0 {
			fmt.Print("[ ")
			for _, zone := range svc.Zones {
				fmt.Printf("%s ", zone)
			}
			fmt.Println("]")
		} else {
			fmt.Printf("[ %s ]\n", rc.Zone().DefaultZone)
		}

		if !svc.ACLs.IsEmpty() {
			fmt.Printf("\tACL Policy: %s\n", svc.ACLs)
		} else {
			fmt.Printf("\tACL Policy: %s\n", rc.ACL().DefaultPolicy)
		}

		fmt.Printf("\tPublish: %v\n", !svc.NoPublish)
		fmt.Printf("\tUse Proxy: %v\n", svc.UseProxy)
		fmt.Println()
	}

	return true
}

func onError(err error) {
	log.Error()
}
