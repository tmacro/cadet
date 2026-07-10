package config

import (
	"encoding/json"

	"github.com/hashicorp/consul/api"
	"github.com/caddyserver/caddy/v2"

	"github.com/tmacro/cadet/pkg/watch"
)

func WatchCaddyConfig(kv *api.KV, key string) watch.Watchable[*caddy.Config] {
	return watch.Adapt(WatchConsulKey(kv, key), func(in *api.KVPair) (*caddy.Config, error) {
		var cfg caddy.Config

		if in == nil || in.Value == nil {
			return &cfg, nil
		}

		err := json.Unmarshal(in.Value, &cfg)
		if err != nil {
			return nil, err
		}

		return &cfg, nil
	})
}
