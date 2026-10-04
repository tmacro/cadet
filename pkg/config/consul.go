package config

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/watch"
)

type TagConfig struct {
	Service   string `json:"service"`
	NoPublish string `json:"no_publish"`
}

func DefaultTagConfig() TagConfig {
	return TagConfig{
		Service:   "cadet",
		NoPublish: "cadet-nopublish",
	}
}

type KeyConfig struct {
	Name  string `json:"name"`
	Zone  string `json:"zone"`
	ACL   string `json:"acl"`
	Proxy string `json:"proxy"`
}

func DefaultKeyConfig() KeyConfig {
	return KeyConfig{
		Name:  "cadet-name",
		Zone:  "cadet-zone",
		ACL:   "cadet-acl",
		Proxy: "cadet-proxy",
	}
}

type ConsulConfig struct {
	Endpoint           *url.URL
	Token              string
	GlobalConfigPrefix string

	Tags TagConfig
	Keys KeyConfig
}

func (c *ConsulConfig) UnmarshalJSON(data []byte) error {
	var raw struct {
		Endpoint           string     `json:"endpoint"`
		Token              string     `json:"token"`
		GlobalConfigPrefix string     `json:"global_config_prefix"`
		Tags               *TagConfig `json:"tags"`
		Keys               *KeyConfig `json:"keys"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if raw.Endpoint != "" {
		ep, err := url.Parse(raw.Endpoint)
		if err != nil {
			return err
		}
		c.Endpoint = ep
	} else {
		c.Endpoint = defaultEndpoint
	}

	if raw.Token != "" {
		c.Token = raw.Token
	} else {
		c.Token = ""
	}

	if raw.GlobalConfigPrefix != "" {
		c.GlobalConfigPrefix = raw.GlobalConfigPrefix
	} else {
		c.GlobalConfigPrefix = "cadet"
	}

	tags := DefaultTagConfig()
	if raw.Tags != nil {
		if raw.Tags.Service != "" {
			tags.Service = raw.Tags.Service
		}
		if raw.Tags.NoPublish != "" {
			tags.NoPublish = raw.Tags.NoPublish
		}
	}

	c.Tags = tags

	keys := DefaultKeyConfig()
	if raw.Keys != nil {
		if raw.Keys.Name != "" {
			keys.Name = raw.Keys.Name
		}
		if raw.Keys.Zone != "" {
			keys.Zone = raw.Keys.Zone
		}
		if raw.Keys.ACL != "" {
			keys.ACL = raw.Keys.ACL
		}
	}

	c.Keys = keys

	return nil
}

var defaultEndpoint, _ = url.Parse("http://localhost:8500")

func DefaultConsulConfig() *ConsulConfig {
	return &ConsulConfig{
		Endpoint:           defaultEndpoint,
		Token:              "",
		GlobalConfigPrefix: "cadet",
		Tags:               DefaultTagConfig(),
		Keys:               DefaultKeyConfig(),
	}
}

func WatchConsulKey(kv *api.KV, key string) watch.Watchable[*api.KVPair] {
	return func(ctx context.Context, lastIndex uint64) (uint64, *api.KVPair, error) {
		opts := &api.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  10 * time.Minute,
		}

		opts = opts.WithContext(ctx)

		for {
			select {
			case <-ctx.Done():
				return lastIndex, nil, nil
			default:
				data, meta, err := kv.Get(key, opts)
				if err != nil {
					return lastIndex, nil, err
				}

				if meta.LastIndex != lastIndex {
					return meta.LastIndex, data, nil
				}
			}
		}
	}
}

func WatchConsulPrefix(kv *api.KV, prefix string) watch.Watchable[[]*api.KVPair] {
	return func(ctx context.Context, lastIndex uint64) (uint64, []*api.KVPair, error) {
		opts := &api.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  10 * time.Minute,
		}

		opts = opts.WithContext(ctx)

		for {
			select {
			case <-ctx.Done():
				return lastIndex, nil, nil
			default:
				pairs, meta, err := kv.List(prefix, opts)
				if err != nil {
					return 0, nil, err
				}

				// Only return if the index has changed
				log.Debug("got watch change ", lastIndex, meta.LastIndex)
				if meta.LastIndex != lastIndex {
					results := make([]*api.KVPair, 0)
					for _, pair := range pairs {
						log.Debug("key ", pair.Key)
						if pair.Key != "" && pair.Value != nil {
							results = append(results, pair)
						}
					}

					return meta.LastIndex, results, nil
				}
			}
		}
	}
}
