package config

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/caddyserver/caddy/v2"

	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/log"
	"github.com/tmacro/cadet/pkg/service"
)

// DashboardConfig controls the web UI. When enabled it is published as a
// service like any other, with a hostname, TLS and ACLs, but served by the
// cadet_dashboard handler instead of a reverse proxy.
type DashboardConfig struct {
	Enabled bool       `json:"enabled"`
	Name    string     `json:"name"`
	Zones   []string   `json:"zones"`
	ACL     acl.Policy `json:"acl"`
}

const (
	DashboardServiceID   = "cadet-dashboard"
	defaultDashboardName = "cadet"
)

func DefaultDashboardConfig() *DashboardConfig {
	return &DashboardConfig{
		Name: defaultDashboardName,
	}
}

// DashboardFromCaddy reads the dashboard config from the cadet app of a caddy
// config, falling back to the default (disabled) config when it isn't set.
func DashboardFromCaddy(cfg *caddy.Config) (*DashboardConfig, error) {
	if cfg == nil || cfg.AppsRaw == nil {
		return DefaultDashboardConfig(), nil
	}

	appRaw, ok := cfg.AppsRaw["cadet"]
	if !ok {
		return DefaultDashboardConfig(), nil
	}

	var app struct {
		Dashboard *DashboardConfig `json:"dashboard"`
	}

	if err := json.Unmarshal(appRaw, &app); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cadet config: %w", err)
	}

	if app.Dashboard == nil {
		return DefaultDashboardConfig(), nil
	}

	return app.Dashboard, nil
}

// Service returns the dashboard as a service config. An empty ACL falls back
// to the global default policy, as it does for real services.
func (d *DashboardConfig) Service() *service.Config {
	name := d.Name
	if name == "" {
		name = defaultDashboardName
	}

	return &service.Config{
		ID:       DashboardServiceID,
		Type:     service.HTTP,
		Name:     name,
		Zones:    d.Zones,
		ACLs:     d.ACL,
		UseProxy: true,
		Handler:  service.DashboardHandler,
	}
}

// AddTo returns services with the dashboard added if it is enabled. A service
// already registered under the dashboard's ID shadows it. services itself is
// never modified.
func (d *DashboardConfig) AddTo(services service.Map) service.Map {
	if !d.Enabled {
		return services
	}

	dashboard := d.Service()
	if _, ok := services[dashboard.ID]; ok {
		log.Warnf("service %s shadows the dashboard, not publishing the dashboard", dashboard.ID)
		return services
	}

	services = maps.Clone(services)
	if services == nil {
		services = make(service.Map)
	}
	services[dashboard.ID] = dashboard

	return services
}
