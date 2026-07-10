package config

import (
	"encoding/json"

	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/watch"
)

type ACL struct {
	Networks      map[string]acl.Network
	Groups        map[string][]string
	DefaultPolicy acl.Policy
	DefaultAction acl.Action
}

func (c *ACL) UnmarshalJSON(data []byte) error {
	var raw struct {
		Networks      map[string]acl.Network `json:"networks"`
		Groups        map[string][]string    `json:"groups"`
		DefaultPolicy acl.Policy             `json:"default_policy"`
		DefaultAction acl.Action             `json:"default_action"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.Networks = raw.Networks
	c.Groups = raw.Groups

	defaultACLCfg := DefaultACLConfig()
	if raw.DefaultPolicy.IsEmpty() {
		c.DefaultPolicy = raw.DefaultPolicy
	} else {
		c.DefaultPolicy = defaultACLCfg.DefaultPolicy
	}

	if raw.DefaultAction.IsEmpty() {
		c.DefaultAction = raw.DefaultAction
	} else {
		c.DefaultAction = defaultACLCfg.DefaultAction
	}
	return nil
}

var DefaultACLPolicy = acl.MustParsePolicy("allow all")

func DefaultACLConfig() *ACL {
	return &ACL{
		Networks:      make(map[string]acl.Network),
		Groups:        make(map[string][]string),
		DefaultPolicy: DefaultACLPolicy,
		DefaultAction: acl.Deny,
	}
}

func WatchACLConfig(kv *api.KV, key string) watch.Watchable[*ACL] {
	return watch.Adapt(WatchConsulKey(kv, key), func(in *api.KVPair) (*ACL, error) {
		if in == nil || in.Value == nil {
			return DefaultACLConfig(), nil
		}

		var cfg ACL

		err := json.Unmarshal(in.Value, &cfg)
		if err != nil {
			return nil, err
		}

		return &cfg, nil
	})
}
