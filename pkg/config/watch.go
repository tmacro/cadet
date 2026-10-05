package config

import (
	"context"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
	"github.com/tmacro/cadet/pkg/watch"
)

var (
	watchIndexes sync.Map
	globalMu     sync.RWMutex

	globalNeedsInit       = true
	globalCatalogServices service.Map
	globalStaticServices  service.Map
	globalACLCfg          *ACL
	globalZoneCfg         *Zone
	globalCaddyCfg        *caddy.Config
)

func getWatchIndex(name string) uint64 {
	val, ok := watchIndexes.Load(name)
	if !ok {
		return 0
	}
	return val.(uint64)
}

func setWatchIndex(name string, idx uint64) uint64 {
	watchIndexes.Store(name, idx)
	return idx
}

func runWatch[T any](ctx context.Context, wg *sync.WaitGroup, name string, w watch.Watchable[T], ch chan<- T, onError ErrorHandler) {
	defer wg.Done()
	defer close(ch)

	lastIndex := getWatchIndex(name)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			nextIndex, value, err := w(ctx, lastIndex)
			if err != nil {
				backoff, exit := onError(err)
				if exit {
					return
				}

				if backoff != 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(backoff):
					}
				}

				continue
			}

			select {
			case <-ctx.Done():
				return
			case ch <- value:
				break
			}

			lastIndex = setWatchIndex(name, nextIndex)
		}
	}
}

type RemoteConfig interface {
	Ready() bool
	Services() service.Map
	ACL() *ACL
	Zone() *Zone
	Caddy() *caddy.Config
}

type ErrorHandler func(error) (time.Duration, bool)

type Watcher struct {
	onChange func(RemoteConfig) bool
	onError  ErrorHandler
}

func NewWatcher(onChange func(RemoteConfig) bool, onError ErrorHandler) *Watcher {
	return &Watcher{
		onChange: onChange,
		onError:  onError,
	}
}

func (w *Watcher) Ready() bool {
	globalMu.
		RLock()
	defer globalMu.RUnlock()
	return !globalNeedsInit
}

func (w *Watcher) Services() service.Map {
	globalMu.RLock()
	defer globalMu.RUnlock()
	svcMap := service.CombineServiceMaps([]service.Map{
		globalCatalogServices,
		globalStaticServices,
	})
	return svcMap
}

func (w *Watcher) ACL() *ACL {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalACLCfg
}

func (w *Watcher) Zone() *Zone {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalZoneCfg
}

func (w *Watcher) Caddy() *caddy.Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalCaddyCfg
}

func (w *Watcher) Run(ctx context.Context, catalog *api.Catalog, kv *api.KV, configPrefix string, extractCfg service.ExtractConfig) error {
	var wg sync.WaitGroup
	defer wg.Wait()

	catalogServiceUpdates := make(chan service.Map, 1)
	staticServiceUpdates := make(chan service.Map, 1)
	aclUpdates := make(chan *ACL, 1)
	zoneUpdates := make(chan *Zone, 1)
	configUpdates := make(chan *caddy.Config, 1)

	catalogServiceWatch := WatchConsulCatalog(catalog, extractCfg)
	staticServiceWatch := WatchStaticServices(kv, configPrefix+"/services/")
	aclWatch := WatchACLConfig(kv, configPrefix+"/acl.json")
	zoneWatch := WatchZoneConfig(kv, configPrefix+"/zone.json")
	configWatch := WatchCaddyConfig(kv, configPrefix+"/caddy.json")

	wg.Add(5)
	go runWatch(ctx, &wg, "catalog_services", catalogServiceWatch, catalogServiceUpdates, w.onError)
	go runWatch(ctx, &wg, "static_services", staticServiceWatch, staticServiceUpdates, w.onError)
	go runWatch(ctx, &wg, "acls", aclWatch, aclUpdates, w.onError)
	go runWatch(ctx, &wg, "zones", zoneWatch, zoneUpdates, w.onError)
	go runWatch(ctx, &wg, "config", configWatch, configUpdates, w.onError)

	needsNotification := false

	globalMu.Lock()
	needsInit := globalNeedsInit
	globalMu.Unlock()

	hasCatalog := false
	hasStaticServices := false
	hasACLCfg := false
	hasZoneCfg := false
	hasCaddyCfg := false

	if needsInit {
		log.Debug("waiting for init")
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case svcMap := <-catalogServiceUpdates:
			log.Debug("got catalog update ", svcMap)
			globalMu.Lock()
			globalCatalogServices = svcMap
			globalMu.Unlock()
			needsNotification = true
			hasCatalog = true

		case svcMap := <-staticServiceUpdates:
			log.Debug("got static service update")
			globalMu.Lock()
			isUpdate := !(len(globalStaticServices) == 0 && len(svcMap) == 0)
			globalStaticServices = svcMap
			globalMu.Unlock()
			if isUpdate {
				needsNotification = true
			}
			hasStaticServices = true

		case aclCfg := <-aclUpdates:
			log.Debug("got acl update")
			globalMu.Lock()
			globalACLCfg = aclCfg
			globalMu.Unlock()
			needsNotification = true
			hasACLCfg = true

		case zoneCfg := <-zoneUpdates:
			log.Debug("got zone update")
			globalMu.Lock()
			globalZoneCfg = zoneCfg
			globalMu.Unlock()
			needsNotification = true
			hasZoneCfg = true

		case caddyCfg := <-configUpdates:
			if caddyCfg != nil {
				log.Debug("got caddy config update")
				globalMu.Lock()
				globalCaddyCfg = caddyCfg
				globalMu.Unlock()
				needsNotification = true
				hasCaddyCfg = true
			}

		case <-time.After(2 * time.Second):
			if !needsNotification {
				break
			}

			needsNotification = false

			if needsInit && hasCatalog && hasStaticServices && hasACLCfg && hasZoneCfg && hasCaddyCfg {
				log.Debug("init finished")
				needsInit = false
				globalMu.Lock()
				globalNeedsInit = false
				globalMu.Unlock()
			}

			if needsInit {
				log.Debug("waiting for init")
				break
			}

			if w.onChange != nil {
				log.Debug("calling onChange")
				if !w.onChange(w) {
					log.Debug("onChange returned false. exiting loop")
					return nil
				}
			}
		}
	}
}

var _ RemoteConfig = (*Watcher)(nil)
