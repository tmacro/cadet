package cadet

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/caddyserver/caddy/v2/modules/caddytls"
	storageconsul "github.com/pteich/caddy-tlsconsul"

	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/config"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
)

var WildcardCIDR *net.IPNet

func init() {
	_, ipnet, err := net.ParseCIDR("0.0.0.0/0")
	if err != nil {
		panic(err)
	}

	WildcardCIDR = ipnet
}

func (plug *Plugin) generateCaddyConfig(conf *caddy.Config, services service.Map, zonfConf *config.Zone, aclConf *config.ACL) error {
	if conf == nil {
		return fmt.Errorf("caddy config is nil")
	}

	if conf.AppsRaw == nil {
		conf.AppsRaw = make(caddy.ModuleMap)
	}

	var plugConfig *Plugin
	if plugConfigRaw, ok := conf.AppsRaw["cadet"]; ok {
		fmt.Println("loading config from kv ", string(plugConfigRaw))
		err := json.Unmarshal(plugConfigRaw, &plugConfig)
		if err != nil {
			return fmt.Errorf("failed to unmarshal cadet config: %w", err)
		}

		if plugConfig.Consul == nil {
			plugConfig.Consul = config.DefaultConsulConfig()
		}

		if plugConfig.AutoReverseProxy == nil {
			plugConfig.AutoReverseProxy = DefaultAutoReverseProxyConfig()
		}

		if plugConfig.Dashboard == nil {
			plugConfig.Dashboard = DefaultDashboardConfig()
		}
	} else {
		return nil
	}

	httpConf := &caddyhttp.App{
		HTTPPort:  plugConfig.AutoReverseProxy.HTTPPort,
		HTTPSPort: plugConfig.AutoReverseProxy.HTTPSPort,
		Servers: map[string]*caddyhttp.Server{
			"http": {
				Listen: []string{
					fmt.Sprintf(":%d", plugConfig.AutoReverseProxy.HTTPPort),
				},
				Routes: caddyhttp.RouteList{},
				AutoHTTPS: &caddyhttp.AutoHTTPSConfig{
					Disabled: true,
				},
				Protocols: plugConfig.AutoReverseProxy.Protocols,
			},
			"tls": {
				Listen: []string{
					fmt.Sprintf(":%d", plugConfig.AutoReverseProxy.HTTPSPort),
				},
				Routes:    caddyhttp.RouteList{},
				Protocols: plugConfig.AutoReverseProxy.Protocols,
			},
		},
	}

	tlsConf := &caddytls.TLS{
		Automation: &caddytls.AutomationConfig{
			Policies: []*caddytls.AutomationPolicy{},
		},
	}

	serviceNames := make([]string, 0, len(services))
	for name := range services {
		serviceNames = append(serviceNames, name)
	}

Outer:
	for _, serviceName := range serviceNames {
		svc, ok := services[serviceName]
		if !ok {
			log.Warnf("service %s not found in service map, skipping", serviceName)
			continue
		}

		if svc.NoPublish || !svc.UseProxy {
			continue
		}

		upstreams := make([]*reverseproxy.Upstream, len(svc.Endpoints))
		for i, endpoint := range svc.Endpoints {
			upstreams[i] = &reverseproxy.Upstream{
				Dial: fmt.Sprintf("%s:%d", endpoint.Address.String(), endpoint.Port),
			}
		}

		if len(upstreams) == 0 {
			log.Warnf("no endpoints found for service %s, skipping", svc.ID)
			continue Outer
		}

		proxyHandler := &reverseproxy.Handler{
			Upstreams: upstreams,
		}

		hostnames := make([]string, 0)
		if len(svc.Zones) == 0 {
			hostnames = append(hostnames, fmt.Sprintf("%s.%s", svc.Name, zonfConf.DefaultZone))
		} else {
			for _, zone := range svc.Zones {
				if !slices.Contains(zonfConf.Zones, zone) {
					log.Warnf("service %s not found in zone %s config", svc.ID, zone)
					continue
				}
				hostnames = append(hostnames, fmt.Sprintf("%s.%s", svc.Name, zone))
			}
		}

		if len(hostnames) == 0 {
			log.Warnf("no hostnames found for service %s, skipping", svc.ID)
			continue Outer
		}

		tlsConf.Automation.Policies = append(tlsConf.Automation.Policies, &caddytls.AutomationPolicy{
			SubjectsRaw: hostnames,
			IssuersRaw:  plugConfig.AutoReverseProxy.TLSIssuers,
		})

		subroute := caddyhttp.Subroute{}

		allowedRanges := []string{}
		deniedRanges := []string{}

		policyToApply := svc.ACLs
		if policyToApply.IsEmpty() {
			policyToApply = aclConf.DefaultPolicy
		}

		for _, control := range policyToApply {
			networks := []acl.Network{}
			if control.Entity == "all" {
				networks = append(networks, acl.Network{WildcardCIDR})
			} else if network, ok := aclConf.Networks[control.Entity]; ok {
				networks = append(networks, network)
			} else {
				if group, ok := aclConf.Groups[control.Entity]; ok {
					for _, groupEntity := range group {
						if net, ok := aclConf.Networks[groupEntity]; ok {
							networks = append(networks, net)
						} else {
							log.Errorf("group entity %s not found in ACL config for service %s", groupEntity, svc.ID)
							continue Outer
						}
					}
				} else {
					log.Errorf("entity %s not found in ACL config for service %s", control.Entity, svc.ID)
					continue Outer
				}
			}

			for _, network := range networks {
				for _, ipRange := range network {
					rangeStr := ipRange.String()
					switch control.Action {
					case acl.Allow:
						if !slices.Contains(allowedRanges, rangeStr) {
							allowedRanges = append(allowedRanges, rangeStr)
						}
					case acl.Deny:
						if !slices.Contains(deniedRanges, rangeStr) {
							deniedRanges = append(deniedRanges, rangeStr)
						}
					default:
						log.Errorf("invalid ACL action %s for service %s", control.Action, svc.ID)
						continue Outer
					}
				}
			}
		}

		if len(deniedRanges) > 0 {
			accessHandler := &caddyhttp.StaticResponse{
				StatusCode: caddyhttp.WeakString("404"),
				Body:       "Not Found",
				Close:      true,
			}

			subroute.Routes = append(subroute.Routes, caddyhttp.Route{
				HandlersRaw: []json.RawMessage{
					caddyconfig.JSONModuleObject(accessHandler, "handler", "static_response", nil),
				},
				MatcherSetsRaw: caddyhttp.RawMatcherSets{
					caddy.ModuleMap{
						"client_ip": caddyconfig.JSON(map[string]any{
							"ranges": deniedRanges,
						}, nil),
					},
				},
				Terminal: true,
			})
		}

		proxyMatchers := caddy.ModuleMap{}

		if aclConf.DefaultAction.IsDeny() && len(allowedRanges) > 0 {
			proxyMatchers["client_ip"] = caddyconfig.JSON(map[string]any{
				"ranges": allowedRanges,
			}, nil)
		}

		subroute.Routes = append(subroute.Routes, caddyhttp.Route{
			HandlersRaw: []json.RawMessage{
				caddyconfig.JSONModuleObject(proxyHandler, "handler", "reverse_proxy", nil),
			},
			MatcherSetsRaw: caddyhttp.RawMatcherSets{proxyMatchers},
			Terminal:       true,
		})

		accessHandler := &caddyhttp.StaticResponse{
			StatusCode: caddyhttp.WeakString("404"),
			Body:       "Not Found",
			Close:      true,
		}

		subroute.Routes = append(subroute.Routes, caddyhttp.Route{
			HandlersRaw: []json.RawMessage{
				caddyconfig.JSONModuleObject(accessHandler, "handler", "static_response", nil),
			},
			Terminal: true,
		})

		servers := []string{"tls"}

		handlersRaw := []json.RawMessage{}
		handlersRaw = append(handlersRaw, caddyconfig.JSONModuleObject(subroute, "handler", "subroute", nil))

		for _, server := range servers {
			httpConf.Servers[server].Routes = append(httpConf.Servers[server].Routes,
				caddyhttp.Route{
					HandlersRaw: handlersRaw,
					MatcherSetsRaw: caddyhttp.RawMatcherSets{
						caddy.ModuleMap{
							"host": caddyconfig.JSON(hostnames, nil),
						},
					},
					Terminal: true,
				},
			)
		}
	}

	conf.AppsRaw["http"] = caddyconfig.JSON(httpConf, nil)
	conf.AppsRaw["tls"] = caddyconfig.JSON(tlsConf, nil)

	storageConf := storageconsul.ConsulStorage{
		Address: plugConfig.Consul.Endpoint.Host,
		Prefix:  plugConfig.Consul.GlobalConfigPrefix + "/certcache",
	}

	if plugConfig.Consul.Endpoint.Scheme == "https" {
		storageConf.TlsEnabled = true
	}

	conf.StorageRaw = caddyconfig.JSONModuleObject(
		storageConf,
		"module",
		"consul",
		nil,
	)

	return nil
}
