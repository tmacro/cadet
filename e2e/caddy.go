package e2e

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/efficientgo/e2e"
	capi "github.com/hashicorp/consul/api"
)

var DefaultProxyConfig = []byte(`{
  "apps": {
    "cadet": {
      "consul": {
        "endpoint": "http://127.0.0.1:8500",
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
            "subjects": ["*.cadet.internal"],
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
}`)

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
	//	sweaters := Inventory{"wool", 17}
	//
	// tmpl, err := template.New("test").Parse("{{.Count}} items are made of {{.Material}}")
	// if err != nil { panic(err) }
	// err = tmpl.Execute(os.Stdout, sweaters)
	// if err != nil { panic(err) }

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

func DeployCaddy(e *e2e.DockerEnvironment, config []byte) (e2e.Runnable, error) {
	configDir := filepath.Join(e.SharedDir(), "proxy")
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "caddy.json")
	err = WriteFile(configPath, config)
	if err != nil {
		return nil, err
	}

	proxy := e.Runnable("proxy").
		WithPorts(
			map[string]int{
				"admin": 2019,
				"http":  80,
				"https": 443,
			}).
		Init(e2e.StartOptions{
			Image: "tmacro/cadet-proxy:latest",
			Command: e2e.Command{
				Cmd: "/usr/local/bin/caddy",
				Args: []string{
					"run",
					"--config",
					configPath,
				},
			},
		})

	if err := e2e.StartAndWaitReady(proxy); err != nil {
		return nil, err
	}

	return proxy, nil
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
		Value: fmt.Append([]byte(`
			{
			    "default_zone": "cadet.internal",
			    "zones": ["cadet.internal", "alternate.internal"]
			}`)),
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
