package cadet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
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

var plugMu *sync.Mutex

func init() {
	plugMu = &sync.Mutex{}
	caddy.RegisterModule(Plugin{})
}

const (
	errorCooldown  = time.Second * 30
	initialBackoff = time.Millisecond * 100
	maxBackoff     = time.Second * 10
)

type Plugin struct {
	Consul           *config.ConsulConfig    `json:"consul"`
	AutoReverseProxy *AutoReverseProxyConfig `json:"auto_reverse_proxy"`
	Dashboard        *DashboardConfig        `json:"dashboard,omitempty"`

	ctx    context.Context
	cancel context.CancelFunc

	catalog *api.Catalog
	kv      *api.KV

	watcher *config.Watcher

	backoffMu   *sync.Mutex
	lastError   time.Time
	lastBackoff time.Duration
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
		backoffMu:        &sync.Mutex{},
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
	extractCfg := service.ExtractConfig{
		ServiceTag:   plug.Consul.Tags.Service,
		NoPublishTag: plug.Consul.Tags.NoPublish,
		ZoneKey:      plug.Consul.Keys.Zone,
		ACLKey:       plug.Consul.Keys.ACL,
		NameKey:      plug.Consul.Keys.Name,
		UseProxyKey:  plug.Consul.Keys.Proxy,
	}

	go supervise.MustRun(plug.ctx, supervise.ServiceFunc(func(ctx context.Context) error {
		plugMu.Lock()
		log.Debug("starting cadet")
		err := plug.watcher.Run(ctx, plug.catalog, plug.kv, plug.Consul.GlobalConfigPrefix, extractCfg)
		log.Debug("cadet exited")
		plugMu.Unlock()
		return err
	}))

	return nil
}

func (plug *Plugin) Stop() error {
	plug.cancel()
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

	var cfg map[string]any
	err = json.Unmarshal(cfgJSON, &cfg)
	if err != nil {
		log.Error("error parsing caddy config", err)
		return false
	}

	cfgPretty, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Errorf("error marshaling acddy config")
		return false
	}

	fmt.Println(string(cfgPretty))

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

func (plug *Plugin) onError(err error) (time.Duration, bool) {
	if !errors.Is(err, context.Canceled) {
		log.Error("plugin got error ", err)
	}

	plug.backoffMu.Lock()
	defer plug.backoffMu.Unlock()

	var backoff time.Duration
	if plug.lastBackoff == 0 || time.Since(plug.lastError) > errorCooldown {
		backoff = initialBackoff
	} else {
		backoff = plug.lastBackoff * 2
		backoff = min(backoff, maxBackoff)
	}

	plug.lastError = time.Now()
	plug.lastBackoff = backoff

	return backoff, false
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
