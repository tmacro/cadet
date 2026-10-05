package e2e

import (
	"time"

	capi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The same driver containers expose a /resolve endpoint that queries CoreDNS
// from inside their network, so the plugin sees a per-network source address.
var (
	resolverA = Resolver{Endpoint: "http://127.0.0.1:28080/resolve"}
	resolverB = Resolver{Endpoint: "http://127.0.0.1:28081/resolve"}
)

const (
	dnsServer = "coredns:53"

	// proxyIP is the address the cadet-proxy service is registered with;
	// services that route through the proxy (the default) resolve to it.
	proxyIP = "172.30.0.3"
	svc1IP  = "172.30.0.4"
	svc2IP  = "172.30.0.5"
)

var _ = Describe("Cadet DNS", func() {
	var (
		consul            *capi.Client
		proxy, svc1, svc2 *TestService
	)

	// direct opts a service out of the proxy so it resolves to its own
	// endpoints rather than the proxy's.
	direct := func(meta map[string]string) map[string]string {
		out := map[string]string{"cadet-proxy": "false"}
		for k, v := range meta {
			out[k] = v
		}
		return out
	}

	BeforeEach(func() {
		var err error
		consul, err = NewConsulClient("http://127.0.0.1:8500")
		Expect(err).NotTo(HaveOccurred())

		Expect(WriteCaddyConfig(consul, FormatCaddyConfig("http://consul:8500"))).To(Succeed())
		Expect(WriteZoneConfig(consul)).To(Succeed())

		// The proxy service must carry the cadet tag to be visible to the
		// plugin, and the no-publish tag so it isn't itself published.
		proxy = &TestService{
			Name:         "cadet-proxy",
			UID:          GenerateUUID(),
			InternalPort: 443,
			InternalIP:   proxyIP,
		}
		Expect(proxy.Register(consul, []string{"cadet", "cadet-no-publish"}, nil)).To(Succeed())

		svc1 = &TestService{
			Name:         "svc1",
			UID:          GenerateUUID(),
			InternalPort: 8080,
			InternalIP:   svc1IP,
		}
		svc2 = &TestService{
			Name:         "svc2",
			UID:          GenerateUUID(),
			InternalPort: 8080,
			InternalIP:   svc2IP,
		}
	})

	AfterEach(func() {
		_ = svc1.Unregister(consul)
		_ = svc2.Unregister(consul)
		_ = proxy.Unregister(consul)
	})

	Context("with services on the primary zone", func() {
		BeforeEach(func() {
			Expect(svc1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			Expect(svc2.Register(consul, []string{"cadet"}, direct(nil))).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("resolves proxied services to the proxy from network A", func() {
			ExpectRecords(resolverA, dnsServer, "svc1.cadet.internal", proxyIP)
		})

		It("resolves direct services to their own endpoint from network B", func() {
			ExpectRecords(resolverB, dnsServer, "svc2.cadet.internal", svc2IP)
		})

		It("does not answer for the service in a zone it isn't in", func() {
			ExpectNoRecords(resolverA, dnsServer, "svc1.alternate.internal")
		})
	})

	Context("with services pinned to a secondary zone", func() {
		BeforeEach(func() {
			meta := map[string]string{"cadet-zone": "alternate.internal"}
			Expect(svc1.Register(consul, []string{"cadet"}, direct(meta))).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("resolves the alternate hostname", func() {
			ExpectRecords(resolverA, dnsServer, "svc1.alternate.internal", svc1IP)
			ExpectRecords(resolverB, dnsServer, "svc1.alternate.internal", svc1IP)
		})

		It("does not resolve the default-zone hostname", func() {
			ExpectNoRecords(resolverA, dnsServer, "svc1.cadet.internal")
		})
	})

	Context("when a service is not eligible to be published", func() {
		When("no service is registered for the hostname", func() {
			It("does not resolve an unknown hostname", func() {
				ExpectNoRecords(resolverA, dnsServer, "nothing.cadet.internal")
			})
		})

		When("the service is tagged with the no-publish tag", func() {
			BeforeEach(func() {
				Expect(svc1.Register(consul, []string{"cadet", "cadet-no-publish"}, direct(nil))).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("does not resolve the service", func() {
				ExpectNoRecords(resolverA, dnsServer, "svc1.cadet.internal")
			})
		})

		When("the service is missing the cadet tag", func() {
			BeforeEach(func() {
				Expect(svc1.Register(consul, nil, direct(nil))).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("does not resolve the service", func() {
				ExpectNoRecords(resolverA, dnsServer, "svc1.cadet.internal")
			})
		})
	})

	Context("when a service overrides its hostname via cadet-name", func() {
		BeforeEach(func() {
			Expect(svc1.Register(consul, []string{"cadet"}, direct(map[string]string{"cadet-name": "aliased"}))).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("resolves the aliased hostname", func() {
			ExpectRecords(resolverA, dnsServer, "aliased.cadet.internal", svc1IP)
		})

		It("does not resolve the original name", func() {
			ExpectNoRecords(resolverA, dnsServer, "svc1.cadet.internal")
		})
	})

	Context("when the service catalog changes", func() {
		BeforeEach(func() {
			Expect(svc1.Register(consul, []string{"cadet"}, direct(nil))).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("stops resolving a service after it is unregistered", func() {
			ExpectRecords(resolverA, dnsServer, "svc1.cadet.internal", svc1IP)

			Expect(svc1.Unregister(consul)).To(Succeed())
			time.Sleep(propagationDelay)

			ExpectNoRecords(resolverA, dnsServer, "svc1.cadet.internal")
		})
	})

	Context("when multiple instances share a service name", func() {
		var inst1, inst2 *TestService

		BeforeEach(func() {
			inst1 = &TestService{Name: "pool", UID: GenerateUUID(), InternalPort: 8080, InternalIP: svc1IP}
			inst2 = &TestService{Name: "pool", UID: GenerateUUID(), InternalPort: 8080, InternalIP: svc2IP}
		})

		AfterEach(func() {
			_ = inst1.Unregister(consul)
			_ = inst2.Unregister(consul)
		})

		It("returns a record for every direct instance", func() {
			Expect(inst1.Register(consul, []string{"cadet"}, direct(nil))).To(Succeed())
			Expect(inst2.Register(consul, []string{"cadet"}, direct(nil))).To(Succeed())
			time.Sleep(propagationDelay)

			ExpectRecords(resolverA, dnsServer, "pool.cadet.internal", svc1IP, svc2IP)
		})

		It("returns only the proxy for proxied instances", func() {
			Expect(inst1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			Expect(inst2.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			time.Sleep(propagationDelay)

			ExpectRecords(resolverA, dnsServer, "pool.cadet.internal", proxyIP)
		})
	})

	Context("with an ACL policy", func() {
		const aclNetworks = `
		"networks" : {
		    "neta": ["172.30.1.0/24"],
		    "netb": ["172.30.2.0/24"]
		},
		"groups" : {
		    "all": ["neta", "netb"]
		}`

		writeACL := func(policy, action string) {
			GinkgoHelper()
			cfg := []byte(`{` + aclNetworks + `,
			"default_policy": "` + policy + `",
			"default_action": "` + action + `"}`)
			Expect(WriteACLConfig(consul, cfg)).To(Succeed())
		}

		AfterEach(func() {
			_ = DeleteACLConfig(consul)
		})

		When("the default action is deny and allow-neta is the default policy", func() {
			BeforeEach(func() {
				writeACL("allow neta", "deny")
				Expect(svc1.Register(consul, []string{"cadet"}, direct(nil))).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("answers network A and not network B", func() {
				ExpectRecords(resolverA, dnsServer, "svc1.cadet.internal", svc1IP)
				ExpectNoRecords(resolverB, dnsServer, "svc1.cadet.internal")
			})
		})

		When("the default action is allow and deny-neta is the default policy", func() {
			BeforeEach(func() {
				writeACL("deny neta", "allow")
				Expect(svc1.Register(consul, []string{"cadet"}, direct(nil))).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("answers network B and not network A", func() {
				ExpectNoRecords(resolverA, dnsServer, "svc1.cadet.internal")
				ExpectRecords(resolverB, dnsServer, "svc1.cadet.internal", svc1IP)
			})
		})

		When("services override the default ACL via meta", func() {
			BeforeEach(func() {
				writeACL("deny *", "deny")
				Expect(svc1.Register(consul, []string{"cadet"}, direct(map[string]string{"cadet-acl": "allow neta"}))).To(Succeed())
				Expect(svc2.Register(consul, []string{"cadet"}, direct(map[string]string{"cadet-acl": "allow netb"}))).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("respects per-service overrides on both networks", func() {
				ExpectRecords(resolverA, dnsServer, "svc1.cadet.internal", svc1IP)
				ExpectNoRecords(resolverA, dnsServer, "svc2.cadet.internal")

				ExpectNoRecords(resolverB, dnsServer, "svc1.cadet.internal")
				ExpectRecords(resolverB, dnsServer, "svc2.cadet.internal", svc2IP)
			})
		})

		When("services allow a group that spans multiple networks", func() {
			BeforeEach(func() {
				writeACL("deny *", "deny")
				Expect(svc1.Register(consul, []string{"cadet"}, direct(map[string]string{"cadet-acl": "allow all"}))).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("answers every network in the group", func() {
				ExpectRecords(resolverA, dnsServer, "svc1.cadet.internal", svc1IP)
				ExpectRecords(resolverB, dnsServer, "svc1.cadet.internal", svc1IP)
			})
		})
	})
})
