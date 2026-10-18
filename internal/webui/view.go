package webui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/tmacro/cadet/pkg/acl"
	"github.com/tmacro/cadet/pkg/config"
	"github.com/tmacro/cadet/pkg/service"
)

// ServiceStatus summarizes how a service is exposed.
type ServiceStatus string

const (
	// StatusRouted services get a reverse-proxy route (and DNS pointing at
	// the proxy).
	StatusRouted ServiceStatus = "proxied"
	// StatusDirect services bypass the proxy; only DNS publishes them,
	// pointing straight at their endpoints.
	StatusDirect ServiceStatus = "direct"
	// StatusUnpublished services carry the no-publish tag.
	StatusUnpublished ServiceStatus = "unpublished"
	// StatusInvalid services are published but can't be served, e.g. they
	// have no endpoints or no usable zones.
	StatusInvalid ServiceStatus = "invalid"
)

type ServiceView struct {
	ID        string
	Name      string
	Type      string
	Status    ServiceStatus
	Hostnames []string
	Endpoints []string
	// Policy is the effective ACL policy; PolicyInherited is true when it
	// comes from the global default rather than the service's own meta.
	Policy          []ControlView
	PolicyInherited bool
	Warnings        []string
}

type ControlView struct {
	Action string
	Entity string
}

func controlViews(policy acl.Policy) []ControlView {
	views := make([]ControlView, 0, len(policy))
	for _, control := range policy {
		views = append(views, ControlView{Action: control.Action.String(), Entity: control.Entity})
	}
	return views
}

type StatusCount struct {
	Status ServiceStatus
	Count  int
}

type ServicesPage struct {
	Ready    bool
	Services []ServiceView
	// Counts holds the number of services per status, in display order,
	// omitting statuses with no services.
	Counts []StatusCount
}

// statusOrder is the display order for statuses: served services first.
var statusOrder = []ServiceStatus{StatusRouted, StatusDirect, StatusInvalid, StatusUnpublished}

type NetworkView struct {
	Name  string
	CIDRs []string
}

type GroupView struct {
	Name     string
	Networks []string
}

type ZoneView struct {
	Name     string
	Default  bool
	Services int
}

type ConfigPage struct {
	Ready       bool
	DefaultZone string
	// DefaultZoneMissing is set when the default zone isn't in Zones.
	DefaultZoneMissing bool
	Zones              []ZoneView
	Networks           []NetworkView
	Groups             []GroupView
	DefaultPolicy      []ControlView
	DefaultAction      string
}

// hostnames mirrors the caddy plugin: explicit zones must exist in the zone
// config, and services without one land in the default zone.
func hostnames(svc *service.Config, zoneCfg *config.Zone) ([]string, []string) {
	var names, warnings []string
	if len(svc.Zones) == 0 {
		if zoneCfg.DefaultZone == "" {
			return nil, []string{"no zone set and no default zone configured"}
		}
		return []string{svc.Name + "." + zoneCfg.DefaultZone}, nil
	}

	for _, zone := range svc.Zones {
		if !slices.Contains(zoneCfg.Zones, zone) {
			warnings = append(warnings, fmt.Sprintf("zone %q is not configured", zone))
			continue
		}
		names = append(names, svc.Name+"."+zone)
	}
	return names, warnings
}

func buildServiceView(svc *service.Config, zoneCfg *config.Zone, aclCfg *config.ACL) ServiceView {
	view := ServiceView{
		ID:   svc.ID,
		Name: svc.Name,
		Type: svc.Type.String(),
	}

	view.Hostnames, view.Warnings = hostnames(svc, zoneCfg)

	for _, ep := range svc.Endpoints {
		view.Endpoints = append(view.Endpoints, ep.String())
	}

	policy := svc.ACLs
	if policy.IsEmpty() {
		policy = aclCfg.DefaultPolicy
		view.PolicyInherited = true
	}
	view.Policy = controlViews(policy)

	switch {
	case svc.NoPublish:
		view.Status = StatusUnpublished
	case len(svc.Endpoints) == 0:
		view.Status = StatusInvalid
		view.Warnings = append(view.Warnings, "no endpoints")
	case len(view.Hostnames) == 0:
		view.Status = StatusInvalid
	case !svc.UseProxy:
		view.Status = StatusDirect
	default:
		view.Status = StatusRouted
	}

	return view
}

func BuildServicesPage(rc config.RemoteConfig) ServicesPage {
	page := ServicesPage{Ready: rc.Ready()}
	if !page.Ready {
		return page
	}

	zoneCfg, aclCfg := rc.Zone(), rc.ACL()
	counts := make(map[ServiceStatus]int)
	for _, svc := range rc.Services() {
		view := buildServiceView(svc, zoneCfg, aclCfg)
		counts[view.Status]++
		page.Services = append(page.Services, view)
	}

	for _, status := range statusOrder {
		if counts[status] > 0 {
			page.Counts = append(page.Counts, StatusCount{Status: status, Count: counts[status]})
		}
	}

	slices.SortFunc(page.Services, func(a, b ServiceView) int {
		return cmp.Or(
			cmp.Compare(slices.Index(statusOrder, a.Status), slices.Index(statusOrder, b.Status)),
			cmp.Compare(a.Name, b.Name),
		)
	})

	return page
}

func BuildConfigPage(rc config.RemoteConfig) ConfigPage {
	page := ConfigPage{Ready: rc.Ready()}
	if !page.Ready {
		return page
	}

	zoneCfg, aclCfg := rc.Zone(), rc.ACL()

	perZone := make(map[string]int)
	for _, svc := range rc.Services() {
		if svc.NoPublish {
			continue
		}
		names, _ := hostnames(svc, zoneCfg)
		for _, zone := range zoneCfg.Zones {
			if slices.Contains(names, svc.Name+"."+zone) {
				perZone[zone]++
			}
		}
	}

	page.DefaultZone = zoneCfg.DefaultZone
	page.DefaultZoneMissing = zoneCfg.DefaultZone != "" && !slices.Contains(zoneCfg.Zones, zoneCfg.DefaultZone)
	for _, zone := range zoneCfg.Zones {
		page.Zones = append(page.Zones, ZoneView{
			Name:     zone,
			Default:  zone == zoneCfg.DefaultZone,
			Services: perZone[zone],
		})
	}

	for _, name := range slices.Sorted(maps.Keys(aclCfg.Networks)) {
		view := NetworkView{Name: name}
		for _, ipnet := range aclCfg.Networks[name] {
			view.CIDRs = append(view.CIDRs, ipnet.String())
		}
		page.Networks = append(page.Networks, view)
	}

	for _, name := range slices.Sorted(maps.Keys(aclCfg.Groups)) {
		page.Groups = append(page.Groups, GroupView{Name: name, Networks: aclCfg.Groups[name]})
	}

	page.DefaultPolicy = controlViews(aclCfg.DefaultPolicy)
	page.DefaultAction = aclCfg.DefaultAction.String()

	return page
}
