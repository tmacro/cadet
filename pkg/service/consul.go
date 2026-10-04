package service

import (
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/hashicorp/consul/api"

	"github.com/tmacro/cadet/pkg/acl"
)

func appendUnique[T comparable](slice []T, item T) []T {
	if !slices.Contains(slice, item) {
		return append(slice, item)
	}
	return slice
}

func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

type ExtractConfig struct {
	ServiceTag   string
	NoPublishTag string
	NameKey      string
	ZoneKey      string
	ACLKey       string
	UseProxyKey  string
}

func FromConsulInstances(tags []string, instances []*api.CatalogService, extrCfg ExtractConfig) (*Config, error) {
	if !slices.Contains(tags, extrCfg.ServiceTag) {
		return nil, nil
	}

	noPublish := false
	if slices.Contains(tags, extrCfg.NoPublishTag) {
		noPublish = true
	}

	svcID := ""
	svcName := ""
	zones := make([]string, 0)
	endpoints := make(EndpointList, 0)
	aclPolicy := ""
	useProxy := true

	for _, instance := range instances {
		if svcID != "" && svcID != instance.ServiceName {
			return nil, fmt.Errorf("multiple service IDs found: %s and %s", svcID, instance.ServiceName)
		}

		svcID = instance.ServiceName

		// instance.Address
		addr := net.ParseIP(instance.ServiceAddress)
		if addr == nil {
			return nil, fmt.Errorf("invalid IP address in Consul instance: %s", instance.ServiceAddress)
		}

		endpoints = append(endpoints, Endpoint{
			Address: addr,
			Port:    uint16(instance.ServicePort),
		})

		nameValue, ok := instance.ServiceMeta[extrCfg.NameKey]
		if ok {
			if svcName != "" && svcName != nameValue {
				return nil, fmt.Errorf("mismatched service names: %s and %s", svcName, nameValue)
			}
			svcName = nameValue
		}

		zoneValue, ok := instance.ServiceMeta[extrCfg.ZoneKey]
		if ok {
			instanceZones := splitTrim(zoneValue, ";")
			for _, zone := range instanceZones {
				if zone != "" {
					zones = appendUnique(zones, zone)
				}
			}
		}

		aclValue, ok := instance.ServiceMeta[extrCfg.ACLKey]
		if ok {
			if aclPolicy != "" && aclPolicy != aclValue {
				return nil, fmt.Errorf("mismatched ACL policies: %s and %s", aclPolicy, aclValue)
			}
			aclPolicy = aclValue
		}

		useProxyValue, ok := instance.ServiceMeta[extrCfg.UseProxyKey]
		if ok {
			if useProxyValue == "false" {
				useProxy = false
			} else if useProxyValue == "true" {
				if !useProxy {
					return nil, fmt.Errorf("conflicting use_proxy values: true and false")
				}
			} else {
				return nil, fmt.Errorf("invalid use_proxy value: %s", useProxyValue)
			}
		}
	}

	if svcName == "" {
		svcName = svcID
	}

	var policy acl.Policy
	var err error
	if aclPolicy != "" {
		policy, err = acl.ParsePolicy(aclPolicy)
		if err != nil {
			return nil, fmt.Errorf("invalid ACL policy: %w", err)
		}
	}

	return &Config{
		ID:        svcID,
		Type:      HTTP,
		Name:      svcName,
		Endpoints: endpoints,
		Zones:     zones,
		ACLs:      policy,
		NoPublish: noPublish,
		UseProxy:  useProxy,
	}, nil
}
