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
| `cadet-proxy`          | `false` points DNS straight at the service instead of the proxy, and skips the proxy route |
| tag `cadet-nopublish`  | skip the whole service even though it carries the `cadet` tag |

Global config lives in consul under `cadet/`:

- `cadet/caddy.json` — the Caddy app config
- `cadet/zone.json`  — default zone and zone list
- `cadet/acl.json`   — ACL networks, groups, and default policy/action
- `cadet/services/`  — static services, one JSON document per key

### Static services

Services that can't register in the consul catalog can be declared in the KV
store instead. Each key under `cadet/services/` holds one service:

```sh
consul kv put cadet/services/nas '{
  "id": "nas",
  "name": "files",
  "endpoints": [
    { "address": "10.10.0.20", "port": 5000 },
    { "address": "10.10.0.21", "port": 5000 }
  ],
  "zones": ["lab.internal"],
  "acls": "allow admin; deny *"
}'
```

| field        | effect                                                                 |
| ------------ | ---------------------------------------------------------------------- |
| `id`         | unique service ID                                                      |
| `name`       | hostname to publish under                                              |
| `endpoints`  | upstreams to proxy to, as `address` (an IP) and `port`                 |
| `zones`      | zones to publish into (default: the default zone)                      |
| `acls`       | ACL policy, same syntax as `cadet-acl` (default: the global policy)    |
| `no_publish` | skip this service (default: `false`)                                   |
| `use_proxy`  | `false` points DNS straight at the endpoints instead of the proxy (default: `true`) |

Services are keyed by `id`, not by KV key. A catalog service with the same ID
takes precedence over a static one.

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

### Dashboard

Setting `dashboard` in the cadet app config publishes the web UI as a
service: it gets a hostname, TLS and ACLs like any other proxied service, but
is served by caddy itself rather than proxied to an upstream.

```json
"dashboard": {
  "enabled": true,
  "name": "cadet",
  "zones": ["cadet.internal"],
  "acl": "allow trusted"
}
```

`name` defaults to `cadet`, `zones` to the default zone and `acl` to the
global default policy, so `{"enabled": true}` publishes the dashboard at
`cadet.<default_zone>`. The service exists only in caddy, so CoreDNS doesn't
answer for it; point the hostname at the proxy yourself (or register a
matching service in consul).

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
network names). `default_policy` is a `;`-separated list of
`<action> <name>` clauses evaluated top-to-bottom, where `*` matches every
source; `default_action` is the fallback when nothing matches. With no
`acl.json`, the policy defaults to `allow *`.

Services can override the global policy per-registration with
`cadet-acl` meta — e.g. `cadet-acl: "allow users; deny lab"`.

## Building

```sh
make help            # list documented targets
make check           # formatting, go.mod, lint and stylesheet checks
make test            # unit tests (everything but ./e2e)
make e2e             # end-to-end suite against a docker compose stack
make caddy           # ./dist/caddy with the cadet plugin baked in
make coredns         # ./dist/coredns with the cadet plugin baked in
make dashboard       # ./dist/cadet-dashboard, the standalone dashboard
make css             # regenerate the dashboard stylesheet (css-watch to rebuild on change)
make caddy-docker    # tmacro/cadet-proxy, tagged with `git describe`
make coredns-docker  # tmacro/cadet-dns, tagged with `git describe`
```

`make fmt` and `make tidy` fix what `check-fmt` and `check-tidy` complain
about; `make check-css` fails when the committed stylesheet is out of date
with the templates. `make e2e-up` / `make e2e-down` start and stop the e2e
stack on their own, which is handy for iterating on single specs.

Pinned tools (xcaddy, golangci-lint, goimports, staticcheck, deadcode,
ginkgo, tailwind) install into `./bin` on first use, or all at once with
`make tools`. Caddy, CoreDNS and Ginkgo versions are read from `go.mod`.
Go, Docker (with buildx and compose) and curl need to be installed.

Images build for `linux/amd64` by default; override `DOCKER_PLATFORMS` for
other targets, e.g. `make caddy-docker DOCKER_PLATFORMS=linux/arm64`.
`make coredns` unpacks the upstream CoreDNS source into `./build` once per
version and builds it against this checkout, using the plugin list in
`images/coredns/plugin.cfg`.

`make clean` removes `./dist` and `./build`; `make fclean` also removes the
tools in `./bin`.
