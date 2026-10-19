package e2e

import (
	"maps"
	"strings"
	"time"

	capi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// settleTimeout bounds how long Settle* wait for the plugins to apply a
// change; propagation normally takes 2-4s (the watcher waits for 2s without
// changes before regenerating config).
const settleTimeout = 30 * time.Second

// registerSentinel registers a uniquely named service pointing at the svc1
// backend, with its own allow-all ACL so no spec's policy can hide it.
func registerSentinel(client *capi.Client, meta map[string]string) *TestService {
	GinkgoHelper()
	uid := strings.ToLower(GenerateUUID())
	sentinel := &TestService{
		Name:         "settle-" + uid,
		UID:          uid,
		InternalPort: 8080,
		InternalIP:   "172.30.0.4",
	}
	m := map[string]string{"cadet-acl": "allow *"}
	maps.Copy(m, meta)
	Expect(sentinel.Register(client, []string{"cadet"}, m)).To(Succeed())
	return sentinel
}

// SettleProxy blocks until the caddy plugin has applied every consul change
// made so far, replacing a fixed sleep. It registers a sentinel service and
// waits for the proxy to route it: the plugin regenerates its config from a
// full catalog snapshot, so once the sentinel is routed, everything
// registered before it is too. KV changes (ACL, zone) written before the
// sentinel arrive well inside the watcher's 2s quiet window, so they land in
// the same or an earlier config.
func SettleProxy(client *capi.Client) {
	GinkgoHelper()
	sentinel := registerSentinel(client, nil)
	defer func() { _ = sentinel.Unregister(client) }()

	Eventually(func() (int, error) {
		code, _, err := driverA.Request("https://caddy:443", sentinel.Name+".cadet.internal")
		return code, err
	}).WithTimeout(settleTimeout).WithPolling(100*time.Millisecond).Should(Equal(200),
		"proxy never routed sentinel %s", sentinel.Name)
}

// SettleDNS is SettleProxy for the coredns plugin: it waits for a direct
// sentinel service to resolve.
func SettleDNS(client *capi.Client) {
	GinkgoHelper()
	sentinel := registerSentinel(client, map[string]string{"cadet-proxy": "false"})
	defer func() { _ = sentinel.Unregister(client) }()

	Eventually(func() ([]string, error) {
		_, answers, err := resolverA.Resolve(dnsServer, sentinel.Name+".cadet.internal")
		return answers, err
	}).WithTimeout(settleTimeout).WithPolling(100*time.Millisecond).Should(ConsistOf(sentinel.InternalIP),
		"coredns never resolved sentinel %s", sentinel.Name)
}
