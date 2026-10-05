package e2e

import (
	capi "github.com/hashicorp/consul/api"
)

func NewConsulClient(addr string) (*capi.Client, error) {
	cfg := capi.DefaultConfig()
	cfg.Address = addr
	return capi.NewClient(cfg)
}
