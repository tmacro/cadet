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

func MergeAutoReverseProxyConfig(a, b *AutoReverseProxyConfig) *AutoReverseProxyConfig {
	cfg := AutoReverseProxyConfig{
		HTTPPort:   a.HTTPPort,
		HTTPSPort:  a.HTTPSPort,
		TLSIssuers: a.TLSIssuers,
		Protocols:  a.Protocols,
	}

	if b.HTTPPort != 0 {
		cfg.HTTPPort = b.HTTPPort
	}

	if b.HTTPSPort != 0 {
		cfg.HTTPSPort = b.HTTPSPort
	}

	if b.TLSIssuers != nil {
		cfg.TLSIssuers = b.TLSIssuers
	}

	if b.Protocols != nil {
		cfg.Protocols = b.Protocols
	}

	return &cfg
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
