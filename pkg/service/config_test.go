package service

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/tmacro/cadet/pkg/acl"
)

func unmarshalConfig(t *testing.T, data string) Config {
	t.Helper()
	var cfg Config
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg
}

func endpointStrings(endpoints EndpointList) []string {
	strs := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		strs = append(strs, ep.String())
	}
	return strs
}

func TestConfigUnmarshal(t *testing.T) {
	cfg := unmarshalConfig(t, `{
		"id": "nas",
		"type": "http",
		"name": "files",
		"endpoints": [
			{"address": "10.10.0.20", "port": 5000},
			{"address": "fd00::21", "port": 5001}
		],
		"zones": ["lab.internal", "cadet.internal"],
		"acls": "allow admin; deny *",
		"no_publish": true,
		"use_proxy": false
	}`)

	if cfg.ID != "nas" {
		t.Errorf("ID = %q, want %q", cfg.ID, "nas")
	}
	if cfg.Type != HTTP {
		t.Errorf("Type = %q, want %q", cfg.Type, HTTP)
	}
	if cfg.Name != "files" {
		t.Errorf("Name = %q, want %q", cfg.Name, "files")
	}

	wantEndpoints := []string{"10.10.0.20:5000", "[fd00::21]:5001"}
	if got := endpointStrings(cfg.Endpoints); !slices.Equal(got, wantEndpoints) {
		t.Errorf("Endpoints = %v, want %v", got, wantEndpoints)
	}

	wantZones := []string{"lab.internal", "cadet.internal"}
	if !slices.Equal(cfg.Zones, wantZones) {
		t.Errorf("Zones = %v, want %v", cfg.Zones, wantZones)
	}

	wantACLs := acl.MustParsePolicy("allow admin; deny *")
	if cfg.ACLs.String() != wantACLs.String() {
		t.Errorf("ACLs = %q, want %q", cfg.ACLs, wantACLs)
	}

	if !cfg.NoPublish {
		t.Error("NoPublish = false, want true")
	}
	if cfg.UseProxy {
		t.Error("UseProxy = true, want false")
	}
}

func TestConfigUnmarshalDefaults(t *testing.T) {
	cfg := unmarshalConfig(t, `{"id": "svc"}`)

	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty", cfg.Name)
	}
	if len(cfg.Endpoints) != 0 {
		t.Errorf("Endpoints = %v, want none", cfg.Endpoints)
	}
	if len(cfg.Zones) != 0 {
		t.Errorf("Zones = %v, want none so the default zone applies", cfg.Zones)
	}
	if !cfg.ACLs.IsEmpty() {
		t.Errorf("ACLs = %q, want empty so the global default applies", cfg.ACLs)
	}
	if cfg.NoPublish {
		t.Error("NoPublish = true, want false")
	}
	if !cfg.UseProxy {
		t.Error("UseProxy = false, want true")
	}
	if cfg.Handler != NoHandler {
		t.Errorf("Handler = %q, want none", cfg.Handler)
	}
}

func TestConfigUnmarshalExplicitFlags(t *testing.T) {
	cfg := unmarshalConfig(t, `{"id": "svc", "no_publish": false, "use_proxy": true}`)

	if cfg.NoPublish {
		t.Error("NoPublish = true, want false")
	}
	if !cfg.UseProxy {
		t.Error("UseProxy = false, want true")
	}
}

func TestConfigUnmarshalIgnoresHandler(t *testing.T) {
	cfg := unmarshalConfig(t, `{"id": "svc", "handler": "dashboard"}`)

	if cfg.Handler != NoHandler {
		t.Errorf("Handler = %q, want none: handlers are internal only", cfg.Handler)
	}
}

func TestConfigUnmarshalInvalid(t *testing.T) {
	tests := map[string]string{
		"invalid ACL policy":       `{"id": "svc", "acls": "maybe admin"}`,
		"invalid endpoint address": `{"id": "svc", "endpoints": [{"address": "nas.local", "port": 5000}]}`,
		"endpoint port overflow":   `{"id": "svc", "endpoints": [{"address": "10.10.0.20", "port": 70000}]}`,
		"non-bool use_proxy":       `{"id": "svc", "use_proxy": "false"}`,
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			if err := json.Unmarshal([]byte(data), &cfg); err == nil {
				t.Fatalf("expected an error, got %+v", cfg)
			}
		})
	}
}
