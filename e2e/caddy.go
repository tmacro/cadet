package e2e

import (
	"bytes"
	"text/template"

	capi "github.com/hashicorp/consul/api"
)

// CaddyConfigTemplate is the Caddy app config loaded from consul at test
// time; ConsulEndpoint is substituted so the proxy can reach consul from
// inside its container network.
const CaddyConfigTemplate = `{
  "apps": {
    "cadet": {
      "consul": {
        "endpoint": "{{ .ConsulEndpoint }}",
        "global_config_prefix": "cadet",
        "tags": {
          "service": "cadet",
          "no_publish": "cadet-no-publish"
        },
        "keys": {
          "name": "cadet-name",
          "zone": "cadet-zone",
          "acl": "cadet-acl"
        }
      },
      "auto_reverse_proxy": {
        "http_port": 80,
        "https_port": 443,
        "protocols": ["h1"],
        "tls_issuers": []
      }
    },
    "tls": {
      "automation": {
        "policies": [
          {
            "subjects": ["*.cadet.internal", "*.alternate.internal"],
            "issuers": [{ "module": "internal" }]
          }
        ]
      }
    }
  },
  "admin": {
    "listen": "0.0.0.0:2019"
  },
  "logging": {
    "logs": {
      "default": {
        "level": "debug"
      }
    }
  }
}`

func FormatCaddyConfig(endpoint string) []byte {
	tmpl := template.Must(template.New("cfg").Parse(CaddyConfigTemplate))
	w := new(bytes.Buffer)
	err := tmpl.Execute(w, map[string]any{
		"ConsulEndpoint": endpoint,
	})
	if err != nil {
		panic(err)
	}
	return w.Bytes()
}

func WriteCaddyConfig(client *capi.Client, cfg []byte) error {
	_, err := client.KV().Put(&capi.KVPair{
		Key:   "cadet/caddy.json",
		Value: cfg,
	}, nil)
	return err
}

func WriteZoneConfig(client *capi.Client) error {
	_, err := client.KV().Put(&capi.KVPair{
		Key: "cadet/zone.json",
		Value: []byte(`{
			"default_zone": "cadet.internal",
			"zones": ["cadet.internal", "alternate.internal"]
		}`),
	}, nil)
	return err
}

func WriteACLConfig(client *capi.Client, cfg []byte) error {
	_, err := client.KV().Put(&capi.KVPair{
		Key:   "cadet/acl.json",
		Value: cfg,
	}, nil)
	return err
}

func DeleteACLConfig(client *capi.Client) error {
	_, err := client.KV().Delete("cadet/acl.json", nil)
	return err
}
