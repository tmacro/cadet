# cadet

Service-discovery-driven routing for your homelab. Register a service in
[Consul](https://www.consul.io/) and cadet turns it into a reverse-proxy
route (and a DNS record) automatically — no restart, no config reload.

Cadet ships as two plugins that both read the same consul data:

- **`plugins/caddy`** — a [Caddy](https://caddyserver.com/) app that watches
  the consul catalog and generates reverse-proxy routes for every tagged
  service, with per-service TLS, hostnames, and ACLs.
- **`plugins/coredns`** — a [CoreDNS](https://coredns.io/) plugin that
  answers queries for the same hostnames the caddy plugin publishes.

## Service model

A service is published if it carries the `cadet` tag in consul. Behavior is
controlled by meta keys on the registration:

| meta key               | effect                                                     |
| ---------------------- | ---------------------------------------------------------- |
| `cadet-name`           | override the hostname (default: service name)              |
| `cadet-zone`           | publish into this zone instead of the default              |
| `cadet-acl`            | ACL policy for this service, overriding the global default |
| tag `cadet-no-publish` | skip this instance even though it carries the `cadet` tag  |

Global config lives in consul under `cadet/`:

- `cadet/caddy.json` — the Caddy app config
- `cadet/zone.json`  — default zone and zone list
- `cadet/acl.json`   — ACL networks, groups, and default policy/action

## Example configs

### Caddy — `/etc/caddy/config.json`

The bootstrap config that tells Caddy where to find consul; everything else
(routes, TLS, upstreams) is generated from the consul catalog at runtime.

```json
{
  "apps": {
    "cadet": {
      "consul": {
        "endpoint": "http://consul:8500"
      },
      "auto_reverse_proxy": {
        "http_port": 80,
        "https_port": 443,
        "protocols": ["h1", "h2"],
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
  }
}
```

Every field under `cadet.consul` has a default; the minimal form is just
`{"endpoint": "..."}`. The full set with its defaults:

```json
{
  "endpoint": "http://localhost:8500",
  "token": "",
  "global_config_prefix": "cadet",
  "tags":  { "service": "cadet",       "no_publish": "cadet-nopublish" },
  "keys":  { "name":    "cadet-name",  "zone":       "cadet-zone",
             "acl":     "cadet-acl",   "proxy":      "cadet-proxy" }
}
```

### Zone — `consul kv put cadet/zone.json ...`

```json
{
  "default_zone": "cadet.internal",
  "zones": ["cadet.internal", "lab.internal"]
}
```

A service without a `cadet-zone` meta key is published at
`<name>.<default_zone>`. Setting `cadet-zone=lab.internal` on a registration
publishes it at `<name>.lab.internal` instead.

### ACL — `consul kv put cadet/acl.json ...`

```json
{
  "networks": {
    "local": ["127.0.0.0/8"],
    "admin": ["10.10.0.0/24"],
    "users": ["10.20.0.0/24"],
    "lab":   ["10.40.0.0/24"]
  },
  "groups": {
    "trusted": ["local", "admin"],
    "all":     ["local", "admin", "users", "lab"]
  },
  "default_policy": "allow trusted",
  "default_action": "deny"
}
```

Requests are matched against `networks` (CIDRs) and `groups` (unions of
network names). `default_policy` is a space-separated list of
`<action> <name>` clauses evaluated top-to-bottom; `default_action` is the
fallback when nothing matches.

Services can override the global policy per-registration with
`cadet-acl` meta — e.g. `cadet-acl: "allow users, deny lab"`.

## Building

```sh
make install-tools   # one-time: xcaddy, ginkgo, linters
make caddy           # ./dist/caddy with the cadet plugin baked in
make caddy-docker    # tmacro/cadet-proxy:latest
make coredns-docker  # tmacro/cadet-dns:latest
```
