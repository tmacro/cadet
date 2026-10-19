package e2e

import (
	capi "github.com/hashicorp/consul/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// dashboardHost is where the caddy plugin publishes the dashboard service,
// per the "dashboard" block of CaddyConfigTemplate.
const dashboardHost = "dashboard.cadet.internal"

func getDashboard(path string) (int, string) {
	GinkgoHelper()
	code, body, err := driverA.Request("https://caddy:443"+path, dashboardHost)
	Expect(err).NotTo(HaveOccurred(), "GET %s%s", dashboardHost, path)
	return code, body
}

var _ = Describe("Cadet dashboard", func() {
	var (
		consul *capi.Client
		svc1   *TestService
	)

	BeforeEach(func() {
		var err error
		consul, err = NewConsulClient("http://127.0.0.1:8500")
		Expect(err).NotTo(HaveOccurred())

		Expect(WriteCaddyConfig(consul, FormatCaddyConfig("http://consul:8500"))).To(Succeed())
		Expect(WriteZoneConfig(consul)).To(Succeed())

		svc1 = &TestService{
			Name:         "svc1",
			UID:          GenerateUUID(),
			InternalPort: 8080,
			InternalIP:   "172.30.0.4",
		}
		Expect(svc1.Register(consul, []string{"cadet"}, map[string]string{"cadet-name": "dashsvc"})).To(Succeed())
		SettleProxy(consul)
	})

	AfterEach(func() {
		_ = svc1.Unregister(consul)
	})

	It("lists routed services with their hostnames and endpoints", func() {
		code, body := getDashboard("/")
		Expect(code).To(Equal(200))
		Expect(body).To(ContainSubstring("dashsvc.cadet.internal"))
		Expect(body).To(ContainSubstring("172.30.0.4:8080"))
	})

	It("shows the global zone config", func() {
		code, body := getDashboard("/config")
		Expect(code).To(Equal(200))
		Expect(body).To(ContainSubstring("cadet.internal"))
		Expect(body).To(ContainSubstring("alternate.internal"))
	})

	It("serves the embedded stylesheet", func() {
		code, _ := getDashboard("/static/css/main.css")
		Expect(code).To(Equal(200))
	})
})
