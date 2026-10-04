package cadet

import (
	"fmt"
	"net/url"

	"github.com/coredns/caddy"
	"github.com/tmacro/cadet/pkg/config"
)

type PluginConfig struct {
	ProxyService string
	Consul       *config.ConsulConfig
}

func DefaultPluginConfig() *PluginConfig {
	return &PluginConfig{
		Consul: config.DefaultConsulConfig(),
	}
}

func ParseConfig(c *caddy.Controller) (*PluginConfig, error) {
	cfg := DefaultPluginConfig()

	n := 0

	for c.Next() {
		if n > 0 {
			return nil, c.Err("unable to load config")
		}

		n++

		for c.NextBlock() {
			val := c.Val()
			args := c.RemainingArgs()

			fmt.Printf("Parsing option '%s' with args %v\n", val, args)
			switch val {
			case "consul_endpoint":
				if len(args) != 1 {
					return nil, c.Err("expected exactly one endpoint")
				}

				ep, err := url.Parse(args[0])
				if err != nil {
					return nil, c.Errf("invalid endpoint '%s': %v", args[0], err)
				}

				cfg.Consul.Endpoint = ep

			case "proxy_service":
				if len(args) != 1 {
					return nil, c.Err("expected exactly one proxy service")
				}
				cfg.ProxyService = args[0]

			case "service_tag":
				if len(args) != 1 {
					return nil, c.Err("expected exactly one watch tag")
				}
				cfg.Consul.Tags.Service = args[0]

			case "zone_key":
				if len(args) != 1 {
					return nil, c.Err("expected exactly one zone key")
				}
				cfg.Consul.Keys.Zone = args[0]

			case "name_key":
				if len(args) != 1 {
					return nil, c.Err("expected exactly one name key")
				}
				cfg.Consul.Keys.Name = args[0]

			case "acl_key":
				if len(args) != 1 {
					return nil, c.Err("expected exactly one ACL key")
				}
				cfg.Consul.Keys.ACL = args[0]

			default:
				return nil, c.Errf("unknown option '%s'", val)
			}
		}
	}

	if cfg.ProxyService == "" {
		return nil, c.Err("proxy_service must be set")
	}

	return cfg, nil
}
