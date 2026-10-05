package e2e

import (
	"time"

	capi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// propagationDelay is how long we wait after mutating consul state before
// expecting Caddy to have reconciled. The underlying watch loop polls at a
// coarse interval; this gives it room to converge.
const propagationDelay = 5 * time.Second

// Drivers are external helpers running on each test network; they issue the
// actual HTTP request from inside their network namespace so we can assert on
// per-source-network ACL behavior.
var (
	driverA = Driver{Endpoint: "http://127.0.0.1:28080/request"}
	driverB = Driver{Endpoint: "http://127.0.0.1:28081/request"}
)

var _ = Describe("Cadet proxy", func() {
	var (
		consul     *capi.Client
		svc1, svc2 *TestService
	)

	BeforeEach(func() {
		var err error
		consul, err = NewConsulClient("http://127.0.0.1:8500")
		Expect(err).NotTo(HaveOccurred())

		Expect(WriteCaddyConfig(consul, FormatCaddyConfig("http://consul:8500"))).To(Succeed())
		Expect(WriteZoneConfig(consul)).To(Succeed())

		// Each spec gets fresh service instances with unique UIDs so stale
		// catalog entries from a previous spec can't bleed in.
		svc1 = &TestService{
			Name:         "svc1",
			UID:          GenerateUUID(),
			Port:         18080,
			InternalPort: 8080,
			InternalIP:   "172.30.0.4",
		}
		svc2 = &TestService{
			Name:         "svc2",
			UID:          GenerateUUID(),
			Port:         18081,
			InternalPort: 8080,
			InternalIP:   "172.30.0.5",
		}
	})

	AfterEach(func() {
		// Best-effort cleanup; swallow errors so one failed deregister does
		// not mask the real spec failure.
		_ = svc1.Unregister(consul)
		_ = svc2.Unregister(consul)
	})

	Context("with services on the primary zone", func() {
		BeforeEach(func() {
			Expect(svc1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			Expect(svc2.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("routes requests from network A to both services", func() {
			ExpectServiceRequest(driverA, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
			ExpectServiceRequest(driverA, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
		})

		It("routes requests from network B to both services", func() {
			ExpectServiceRequest(driverB, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
			ExpectServiceRequest(driverB, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
		})
	})

	Context("with services pinned to a secondary zone", func() {
		BeforeEach(func() {
			meta := map[string]string{"cadet-zone": "alternate.internal"}
			Expect(svc1.Register(consul, []string{"cadet"}, meta)).To(Succeed())
			Expect(svc2.Register(consul, []string{"cadet"}, meta)).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("serves the alternate hostname from network A", func() {
			ExpectServiceRequest(driverA, "https://caddy:443", "svc1.alternate.internal", 200, "svc1")
			ExpectServiceRequest(driverA, "https://caddy:443", "svc2.alternate.internal", 200, "svc2")
		})

		It("serves the alternate hostname from network B", func() {
			ExpectServiceRequest(driverB, "https://caddy:443", "svc1.alternate.internal", 200, "svc1")
			ExpectServiceRequest(driverB, "https://caddy:443", "svc2.alternate.internal", 200, "svc2")
		})
	})

	Context("when a service is not eligible to be published", func() {
		When("no service is registered for the hostname", func() {
			It("does not publish an unknown hostname", func() {
				ExpectHostUnreachable(driverA, "https://caddy:443", "nothing.cadet.internal")
			})
		})

		When("the service is tagged with the no-publish tag", func() {
			BeforeEach(func() {
				Expect(svc1.Register(consul, []string{"cadet", "cadet-no-publish"}, nil)).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("does not publish the service", func() {
				ExpectHostUnreachable(driverA, "https://caddy:443", "svc1.cadet.internal")
			})
		})

		When("the service is missing the cadet tag", func() {
			BeforeEach(func() {
				// Nothing but the per-spec UID tag — so the plugin's
				// `service` tag filter rejects this registration.
				Expect(svc1.Register(consul, nil, nil)).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("does not publish the service", func() {
				ExpectHostUnreachable(driverA, "https://caddy:443", "svc1.cadet.internal")
			})
		})
	})

	Context("when a service overrides its hostname via cadet-name", func() {
		BeforeEach(func() {
			Expect(svc1.Register(consul, []string{"cadet"}, map[string]string{"cadet-name": "aliased"})).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("serves the service at the aliased hostname", func() {
			ExpectServiceRequest(driverA, "https://caddy:443", "aliased.cadet.internal", 200, "svc1")
		})

		It("does not serve the service at its original name", func() {
			ExpectHostUnreachable(driverA, "https://caddy:443", "svc1.cadet.internal")
		})
	})

	Context("when the service catalog changes", func() {
		BeforeEach(func() {
			Expect(svc1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			time.Sleep(propagationDelay)
		})

		It("stops routing to a service after it is unregistered", func() {
			// Confirm reachable first so a failure here is clearly a baseline
			// problem rather than a propagation issue.
			ExpectServiceRequest(driverA, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")

			Expect(svc1.Unregister(consul)).To(Succeed())
			time.Sleep(propagationDelay)

			ExpectHostUnreachable(driverA, "https://caddy:443", "svc1.cadet.internal")
		})
	})

	Context("when multiple instances share a service name", func() {
		// Two instances registered under the same Name pointing at the two
		// different whoami backends; the proxy should treat them as a single
		// upstream pool and distribute requests across both.
		var inst1, inst2 *TestService

		BeforeEach(func() {
			inst1 = &TestService{
				Name:         "pool",
				UID:          GenerateUUID(),
				InternalPort: 8080,
				InternalIP:   "172.30.0.4", // svc1 whoami backend
			}
			inst2 = &TestService{
				Name:         "pool",
				UID:          GenerateUUID(),
				InternalPort: 8080,
				InternalIP:   "172.30.0.5", // svc2 whoami backend
			}
			Expect(inst1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			Expect(inst2.Register(consul, []string{"cadet"}, nil)).To(Succeed())
			time.Sleep(propagationDelay)
		})

		AfterEach(func() {
			_ = inst1.Unregister(consul)
			_ = inst2.Unregister(consul)
		})

		It("distributes requests across both backends", func() {
			// Request enough times that the odds of a correct load balancer
			// never picking one of the two backends is negligible.
			const requests = 20
			seen := make(map[string]int)
			for range requests {
				code, body, err := driverA.Request("https://caddy:443", "pool.cadet.internal")
				Expect(err).NotTo(HaveOccurred())
				Expect(code).To(Equal(200))
				seen[body]++
			}
			Expect(seen).To(HaveKey("svc1"), "expected to see responses from the svc1 backend")
			Expect(seen).To(HaveKey("svc2"), "expected to see responses from the svc2 backend")
		})
	})

	Context("with an ACL policy", func() {
		// aclNetworks is the common network definition used by every ACL spec;
		// the policy / action lines are what each spec varies.
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
				Expect(svc1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
				Expect(svc2.Register(consul, []string{"cadet"}, nil)).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("allows network A and denies network B", func() {
				ExpectServiceRequest(driverA, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
				ExpectServiceRequest(driverA, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")

				ExpectServiceRequest(driverB, "https://caddy:443", "svc1.cadet.internal", 404, "Not Found")
				ExpectServiceRequest(driverB, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")
			})
		})

		When("the default action is allow and deny-neta is the default policy", func() {
			BeforeEach(func() {
				writeACL("deny neta", "allow")
				Expect(svc1.Register(consul, []string{"cadet"}, nil)).To(Succeed())
				Expect(svc2.Register(consul, []string{"cadet"}, nil)).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("denies network A and allows network B", func() {
				ExpectServiceRequest(driverA, "https://caddy:443", "svc1.cadet.internal", 404, "Not Found")
				ExpectServiceRequest(driverA, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")

				ExpectServiceRequest(driverB, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
				ExpectServiceRequest(driverB, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
			})
		})

		When("services override the default ACL via tags", func() {
			BeforeEach(func() {
				writeACL("deny all", "deny")
				Expect(svc1.Register(consul, []string{"cadet"}, map[string]string{"cadet-acl": "allow neta"})).To(Succeed())
				Expect(svc2.Register(consul, []string{"cadet"}, map[string]string{"cadet-acl": "allow netb"})).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("respects per-service overrides on both networks", func() {
				ExpectServiceRequest(driverA, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
				ExpectServiceRequest(driverA, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")

				ExpectServiceRequest(driverB, "https://caddy:443", "svc1.cadet.internal", 404, "Not Found")
				ExpectServiceRequest(driverB, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
			})
		})

		When("services allow a group that spans multiple networks", func() {
			BeforeEach(func() {
				writeACL("deny all", "deny")
				Expect(svc1.Register(consul, []string{"cadet"}, map[string]string{"cadet-acl": "allow all"})).To(Succeed())
				Expect(svc2.Register(consul, []string{"cadet"}, map[string]string{"cadet-acl": "allow netb"})).To(Succeed())
				time.Sleep(propagationDelay)
			})

			It("allows the group member from every network in it", func() {
				ExpectServiceRequest(driverA, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
				ExpectServiceRequest(driverA, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")

				ExpectServiceRequest(driverB, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
				ExpectServiceRequest(driverB, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
			})
		})
	})
})
