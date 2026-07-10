package config

import (
	"encoding/json"

	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/watch"
)

type Zone struct {
	DefaultZone string   `json:"default_zone,omitempty"`
	Zones       []string `json:"zones,omitempty"`
}

func DefaultZoneConfig() *Zone {
	return &Zone{
		Zones: []string{},
	}
}

func WatchZoneConfig(kv *api.KV, key string) watch.Watchable[*Zone] {
	return watch.Adapt(WatchConsulKey(kv, key), func(in *api.KVPair) (*Zone, error) {
		if in == nil || in.Value == nil {
			return DefaultZoneConfig(), nil
		}

		var cfg Zone
		err := json.Unmarshal(in.Value, &cfg)
		if err != nil {
			return nil, err
		}

		return &cfg, nil
	})
}
