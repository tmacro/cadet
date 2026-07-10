package consul

import (
	"github.com/hashicorp/consul/api"
)

func CreateClient(scheme, endpoint, token string) (c *api.Catalog, kv *api.KV, err error) {
	cfg := api.DefaultConfig()
	cfg.Address = endpoint
	if token != "" {
		cfg.Token = token
	}

	if scheme == "https" {
		cfg.Scheme = "https"
	}

	client, err := api.NewClient(cfg)

	if err != nil {
		return
	}

	c = client.Catalog()
	kv = client.KV()
	return
}