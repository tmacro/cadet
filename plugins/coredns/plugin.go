package cadet

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/config"
	"github.com/tmacro/cadet/pkg/consul"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
	"github.com/tmacro/cadet/pkg/supervise"

	clog "github.com/coredns/coredns/plugin/pkg/log"
)

func init() {
	log.SetLogger(log.NewClogShim(clog.NewWithPlugin("cadet")))
	plugin.Register("cadet", setupPlugin)
}

func setupPlugin(c *caddy.Controller) error {
	plugConfig, err := ParseConfig(c)
	if err != nil {
		return err
	}

	plug, err := NewPlugin(plugConfig)
	if err != nil {
		return err
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		plug.Next = next
		return plug
	})

	extractCfg := service.ExtractConfig{
		ServiceTag:   plugConfig.Consul.Tags.Service,
		NoPublishTag: plugConfig.Consul.Tags.NoPublish,
		ZoneKey:      plugConfig.Consul.Keys.Zone,
		ACLKey:       plugConfig.Consul.Keys.ACL,
		NameKey:      plugConfig.Consul.Keys.Name,
		UseProxyKey:  plugConfig.Consul.Keys.Proxy,
	}

	c.OnStartup(func() error {
		catalog, kv, err := consul.CreateClient(
			plugConfig.Consul.Endpoint.Scheme,
			plugConfig.Consul.Endpoint.Host,
			plugConfig.Consul.Token,
		)

		if err != nil {
			log.Errorf("failed to create consul client: %v", err)
			return err
		}

		go supervise.MustRun(plug.ctx, supervise.ServiceFunc(func(ctx context.Context) error {
			return plug.watcher.Run(ctx, catalog, kv, plugConfig.Consul.GlobalConfigPrefix, extractCfg)
		}))

		plug.waitForReady()
		return nil
	})

	c.OnShutdown(func() error {
		log.Debug("shutting down cadet plugin")
		plug.shutdown()
		return nil
	})

	return nil
}

type Plugin struct {
	Next plugin.Handler

	cfg *PluginConfig

	rc    config.RemoteConfig
	ready chan struct{}

	ctx    context.Context
	cancel context.CancelFunc

	watcher *config.Watcher

	zoneMap ZoneMap
	zoneMu  sync.RWMutex
}

func NewPlugin(cfg *PluginConfig) (*Plugin, error) {

	ctx, cancel := context.WithCancel(context.Background())

	plug := &Plugin{
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
		cfg:    cfg,
	}

	watcher := config.NewWatcher(plug.onChange, plug.onError)
	plug.watcher = watcher

	return plug, nil
}

var _ plugin.Handler = (*Plugin)(nil)

func (plug *Plugin) Name() string { return "cadet" }

func (plug *Plugin) Ready() bool {
	select {
	case <-plug.ready:
		return true
	default:
	}
	return false
}

func (plug *Plugin) waitForReady() bool {
	select {
	case <-plug.ctx.Done():
		return false
	case <-plug.ready:
		return true
	}
}

func (plug *Plugin) shutdown() {
	plug.cancel()
}

type ZoneEntry struct {
	ServiceID string
	Policy    acl.Policy
	Endpoints service.EndpointList
}

type Zone map[string]*ZoneEntry

type ZoneMap map[string]Zone

func (plug *Plugin) onChange(rc config.RemoteConfig) bool {
	zoneCfg := rc.Zone()
	zoneMap := make(ZoneMap)
	for _, zone := range zoneCfg.Zones {
		zoneMap[zone] = make(Zone)
	}

	svcs := rc.Services()
	for _, svc := range svcs {
		if svc.NoPublish {
			continue
		}

		endpoints := svc.Endpoints
		if svc.UseProxy {
			proxySvc, ok := svcs[plug.cfg.ProxyService]
			if !ok {
				log.Debugf("proxy service %s not found for service %s", plug.cfg.ProxyService, svc.ID)
				continue
			}

			endpoints = proxySvc.Endpoints
		}

		if len(svc.Zones) > 0 {
			for _, zone := range svc.Zones {
				if slices.Contains(zoneCfg.Zones, zone) {
					zoneMap[zone][svc.Name] = &ZoneEntry{
						ServiceID: svc.ID,
						Policy:    svc.ACLs,
						Endpoints: endpoints,
					}
				}
			}
		}
	}

	plug.zoneMu.Lock()
	plug.zoneMap = zoneMap
	plug.zoneMu.Unlock()

	return true
}

func (plug *Plugin) onError(err error) (time.Duration, bool) {
	return time.Millisecond * 100, false
}

func (plug *Plugin) getZoneEntry(z, r string) (*ZoneEntry, bool) {
	plug.zoneMu.RLock()
	defer plug.zoneMu.RUnlock()

	zone, ok := plug.zoneMap[z]
	if !ok {
		return nil, false
	}

	svc, ok := zone[r]
	if !ok {
		return nil, false
	}

	return svc, true
}

func (plug *Plugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	if !plug.Ready() {
		return dns.RcodeServerFailure, errors.New("plugin not ready")
	}

	req := request.Request{W: w, Req: r}

	qname := req.Name()
	qtype := req.QType()

	log.Debugf("Received query for %s from %s", qname, req.IP())
	zone, record := GetZoneAndRecord(plug.watcher.Zone().Zones, qname)
	if zone == "" || record == "@" {
		log.Debugf("Name %s not in configured zones %s, passing to next plugin", qname, plug.watcher.Zone().Zones)
		return plugin.NextOrFailure(plug.Name(), plug.Next, ctx, w, r)
	}

	log.Debugf("Received new request for zone '%s' and record '%s' with code '%s'", zone, record, dns.TypeToString[qtype])

	entry, ok := plug.getZoneEntry(zone, record)
	if !ok {
		return plugin.NextOrFailure(plug.Name(), plug.Next, ctx, w, r)
	}

	log.Debugf("Found service %s in zone %s with endpoints: %v", entry.ServiceID, zone, entry.Endpoints)

	ip := net.ParseIP(req.IP())
	aclCfg := plug.watcher.ACL()

	action := plug.evalACL(entry, &ip, aclCfg)
	if action.IsDeny() {
		log.Debugf("ACL denied for %s in zone %s", qname, zone)
		return plugin.NextOrFailure(plug.Name(), plug.Next, ctx, w, r)
	}

	log.Debugf("ACL allowed for %s in zone %s", qname, zone)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.RecursionAvailable = false

	for _, ep := range entry.Endpoints {
		rr := &dns.A{
			Hdr: dns.RR_Header{Name: dns.Fqdn(qname), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0},
			A:   ep.Address,
		}

		m.Answer = append(m.Answer, rr)
	}

	err := w.WriteMsg(m)
	if err != nil {
		log.Errorf("Failed to write response for %s: %v", qname, err)
		return dns.RcodeServerFailure, err
	}

	return dns.RcodeSuccess, nil
}

func (plug *Plugin) evalACL(entry *ZoneEntry, ip *net.IP, aclConfig *config.ACL) acl.Action {
	policy := entry.Policy
	if entry.Policy.IsEmpty() {
		policy = aclConfig.DefaultPolicy
	}

	if !policy.IsValid() {
		log.Errorf("acl policy is invalid: %s", policy.String())
	}

	network, ok := aclConfig.MatchNetwork(ip)
	if !ok {
		log.Debugf("no matching network for: %s", ip.String())
	}

	action, matched, err := policy.Eval(network)
	if err != nil {
		log.Errorf("error evaluating policy: %s", policy.String())
		return acl.Deny
	}

	if matched {
		log.Debugf("acl match for network %s: %s", network, action.String())
		return action
	}

	log.Debugf("no acl match for network: %s", network)

	groups, ok := aclConfig.MatchGroups(network)
	if !ok {
		log.Errorf("no matching groups for network: %s", network)
		return aclConfig.DefaultAction
	}

	for _, group := range groups {
		action, matched, err := policy.Eval(group)
		if err != nil {
			log.Errorf("error evaluating policy: %s", policy.String())
			return acl.Deny
		}

		if matched {
			log.Debugf("acl match for group %s: %s", group, action.String())
			return action
		}
	}

	log.Debugf("no acl match for network: %s or groups: %v", network, groups)
	return aclConfig.DefaultAction
}

func GetZoneAndRecord(zones []string, qname string) (string, string) {
	qname = strings.TrimSuffix(dns.Fqdn(qname), ".")

	for _, zone := range zones {
		record, present := strings.CutSuffix(qname, zone)
		if present {
			if record == "." {
				return zone, "@"
			}

			return zone, strings.TrimSuffix(record, ".")
		}
	}

	return "", ""
}
