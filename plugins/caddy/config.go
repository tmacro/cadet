package cadet

import (
	"encoding/json"

	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/service"
)

type AutoReverseProxyConfig struct {
	HTTPPort   int               `json:"http_port"`
	HTTPSPort  int               `json:"https_port"`
	TLSIssuers []json.RawMessage `json:"tls_issuers" caddy:"namespace=tls.issuance inline_key=module"`
	Protocols  []string          `json:"protocols"`
}

func DefaultAutoReverseProxyConfig() *AutoReverseProxyConfig {
	return &AutoReverseProxyConfig{
		HTTPPort:   80,
		HTTPSPort:  443,
		TLSIssuers: []json.RawMessage{},
		Protocols:  []string{"h1", "h2", "h3"},
	}
}

// DashboardConfig controls the web UI. When enabled it is published as a
// service like any other, with a hostname, TLS and ACLs, but served by the
// cadet_dashboard handler instead of a reverse proxy.
type DashboardConfig struct {
	Enabled bool       `json:"enabled"`
	Name    string     `json:"name"`
	Zones   []string   `json:"zones"`
	ACL     acl.Policy `json:"acl"`
}

const (
	dashboardServiceID   = "cadet-dashboard"
	defaultDashboardName = "cadet"
)

func DefaultDashboardConfig() *DashboardConfig {
	return &DashboardConfig{
		Name: defaultDashboardName,
	}
}

// Service returns the dashboard as a service config. An empty ACL falls back
// to the global default policy, as it does for real services.
func (d *DashboardConfig) Service() *service.Config {
	name := d.Name
	if name == "" {
		name = defaultDashboardName
	}

	return &service.Config{
		ID:       dashboardServiceID,
		Type:     service.HTTP,
		Name:     name,
		Zones:    d.Zones,
		ACLs:     d.ACL,
		UseProxy: true,
		Handler:  service.DashboardHandler,
	}
}

type ProxyService struct {
	Name      string     `json:"name"`
	Hostnames []string   `json:"hostnames"`
	Upstreams []string   `json:"upstreams"`
	ACL       acl.Policy `json:"acl"`
}

type ProxyServer struct {
	Port     int            `json:"port"`
	Services []ProxyService `json:"servers"`
}

type ProxyConfig struct {
	Servers map[string]ProxyServer
}
