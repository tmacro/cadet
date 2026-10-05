package cadet

import (
	"encoding/json"

	"github.com/tmacro/cadet/pkg/acl"
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

type DashboardConfig struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

func DefaultDashboardConfig() *DashboardConfig {
	return &DashboardConfig{
		Port: 10443,
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
