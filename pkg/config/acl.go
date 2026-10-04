package config

import (
	"encoding/json"
	"fmt"
	"net"
	"slices"

	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/log"
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
	if !raw.DefaultPolicy.IsEmpty() {
		c.DefaultPolicy = raw.DefaultPolicy
	} else {
		c.DefaultPolicy = defaultACLCfg.DefaultPolicy
	}

	if !raw.DefaultAction.IsEmpty() {
		c.DefaultAction = raw.DefaultAction
	} else {
		c.DefaultAction = defaultACLCfg.DefaultAction
	}
	return nil
}

func (c *ACL) MatchNetwork(ip *net.IP) (string, bool) {
	for name, network := range c.Networks {
		if network.Matches(ip) {
			return name, true
		}
	}

	return "", false
}

func (c *ACL) MatchGroups(network string) ([]string, bool) {
	matches := make([]string, 0)
	for group, networks := range c.Groups {
		if slices.Contains(networks, network) {
			matches = append(matches, group)
		}
	}

	if len(matches) > 0 {
		return matches, true
	}

	return nil, false
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
			log.Debug("using default acl config")
			return DefaultACLConfig(), nil
		}

		fmt.Println(string(in.Value))

		var cfg ACL
		err := json.Unmarshal(in.Value, &cfg)
		if err != nil {
			return nil, err
		}

		return &cfg, nil
	})
}
