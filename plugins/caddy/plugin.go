package cadet

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/tmacro/cadet/pkg/supervise"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"

	"github.com/tmacro/cadet/pkg/config"
	"github.com/tmacro/cadet/pkg/consul"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
)

func init() {
	caddy.RegisterModule(Plugin{})
}

type Plugin struct {
	Consul           *config.ConsulConfig    `json:"consul"`
	AutoReverseProxy *AutoReverseProxyConfig `json:"auto_reverse_proxy"`
	Dashboard        *DashboardConfig        `json:"dashboard,omitempty"`

	ctx    context.Context
	cancel context.CancelFunc

	catalog *api.Catalog
	kv      *api.KV

	watcher *config.Watcher
}

var (
	_ caddy.Module       = (*Plugin)(nil)
	_ caddy.App          = (*Plugin)(nil)
	_ caddy.Provisioner  = (*Plugin)(nil)
	_ caddy.Validator    = (*Plugin)(nil)
	_ caddy.CleanerUpper = (*Plugin)(nil)
	//  _ caddyfile.Unmarshaler = (*Plugin)(nil)
)

func NewPlugin() *Plugin {
	return &Plugin{
		Consul:           config.DefaultConsulConfig(),
		AutoReverseProxy: DefaultAutoReverseProxyConfig(),
		Dashboard:        DefaultDashboardConfig(),
	}
}

func (plug Plugin) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "cadet",
		New: func() caddy.Module { return NewPlugin() },
	}
}

func (plug *Plugin) Provision(ctx caddy.Context) error {
	log.SetLogger(caddy.Log().Named("cadet").Sugar())

	var err error
	plug.catalog, plug.kv, err = consul.CreateClient(plug.Consul.Endpoint.Scheme, plug.Consul.Endpoint.Host, plug.Consul.Token)
	if err != nil {
		log.Errorf("failed to create consul client: %v", err)
		return err
	}

	plug.ctx, plug.cancel = context.WithCancel(context.Background())
	plug.watcher = config.NewWatcher(plug.onChange, plug.onError)
	return nil
}

func (plug *Plugin) Start() error {
	log.Debug("starting cadet")
	extractCfg := service.ExtractConfig{
		ServiceTag:   plug.Consul.Tags.Service,
		NoPublishTag: plug.Consul.Tags.NoPublish,
		ZoneKey:      plug.Consul.Keys.Zone,
		ACLKey:       plug.Consul.Keys.ACL,
		NameKey:      plug.Consul.Keys.Name,
		UseProxyKey:  plug.Consul.Keys.Proxy,
	}

	go supervise.MustRun(plug.ctx, supervise.ServiceFunc(func(ctx context.Context) error {
		return plug.watcher.Run(ctx, plug.catalog, plug.kv, plug.Consul.GlobalConfigPrefix, extractCfg)
	}))

	return nil
}

func (plug *Plugin) Stop() error {
	plug.cancel()
	log.Debug("cadet exited")
	return nil
}

func (plug *Plugin) Cleanup() error {
	return nil
}

func (plug *Plugin) Validate() error {
	return nil
}

func (plug *Plugin) onChange(rc config.RemoteConfig) bool {
	printServices(rc.Services())

	caddyConf := rc.Caddy()
	err := plug.generateCaddyConfig(caddyConf, rc.Services(), rc.Zone(), rc.ACL())
	if err != nil {
		log.Error("error generating caddy config", err)
		return false
	}

	cfgJSON := caddyconfig.JSON(caddyConf, nil)
	log.Debugf("generated Caddy config: %s", cfgJSON)

	n := rand.Intn(1500)
	log.Debugf("sleeping %d ms before applying new configuration", n)

	select {
	case <-plug.ctx.Done():
		return false
	case <-time.After(time.Duration(n) * time.Millisecond):
		log.Info("applying new configuration")
		err = caddy.Load(cfgJSON, false)
		if err != nil {
			log.Errorf("Unable to load conf config: %s", err)
		}

		if plug.ctx.Err() != nil {
			log.Debugf("context cancelled, stopping event loop")
			return false
		}
	}

	return true
}

func (plug *Plugin) onError(err error) {
	log.Error("plugin got error", err)
}

func printServices(svcMap service.Map) {
	for _, svc := range svcMap {
		fmt.Println("Service:", svc.ID)
		fmt.Printf("\tType: %s\n", svc.Type)
		fmt.Printf("\tName: %s\n", svc.Name)
		fmt.Println("\tEndpoints:")
		for _, ep := range svc.Endpoints {
			fmt.Printf("\t\t%s\n", ep)
		}
		fmt.Println("\tZones:")
		for _, zone := range svc.Zones {
			fmt.Printf("\t\t%s\n", zone)
		}
		fmt.Printf("\tACL Policy: %s\n", svc.ACLs)
		fmt.Printf("\tPublish: %v\n", !svc.NoPublish)
		fmt.Printf("\tUse Proxy: %v\n", svc.UseProxy)
		fmt.Println()
	}
}
