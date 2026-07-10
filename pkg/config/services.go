package config

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/watch"
	"github.com/tmacro/cadet/pkg/service"
)

func WatchConsulCatalog(catalog *api.Catalog, cfg service.ExtractConfig) watch.Watchable[service.Map] {
	return func(ctx context.Context, lastIndex uint64) (uint64, service.Map, error) {
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
				svcs, meta, err := catalog.Services(opts)
				if err != nil {
					return lastIndex, nil, err
				}

				// Only return if the index has changed
				if meta.LastIndex != lastIndex {
					svcMap := make(service.Map)
					for name, tags := range svcs {
						if !slices.Contains(tags, cfg.ServiceTag) {
							continue
						}

						instances, _, err := catalog.Service(name, "", nil)
						if err != nil {
							return lastIndex, nil, err
						}

						if len(instances) == 0 {
							continue
						}

						svc, err := service.FromConsulInstances(tags, instances, cfg)
						if err != nil {
							return lastIndex, nil, err
						}
						svcMap[name] = svc
					}

					return meta.LastIndex, svcMap, nil
				}
			}
		}
	}
}

func WatchStaticServices(kv *api.KV, prefix string) watch.Watchable[service.Map] {
	return watch.Adapt(WatchConsulPrefix(kv, prefix), func(in []*api.KVPair) (service.Map, error) {
		svcMap := make(service.Map)
		for _, pair := range in {
			var svc service.Config
			err := json.Unmarshal(pair.Value, &svc)
			if err != nil {
				return nil, err
			}

			svcMap[svc.ID] = &svc
		}

		return svcMap, nil
	})
}
