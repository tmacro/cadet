package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	capi "github.com/hashicorp/consul/api"
	"github.com/jxskiss/base62"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// GenerateUUID returns a short base62-encoded uuid, used as a per-spec
// service tag so stale catalog entries can't collide across specs.
func GenerateUUID() string {
	uuid, _ := uuid.New().MarshalBinary()
	id := base62.EncodeToString(uuid)
	// Drop base62 padding.
	return id[:len(id)-6]
}

// TestService is a service we register in consul to drive proxy behavior;
// InternalIP/InternalPort are the upstream the proxy will route to, Port is
// the host-side port the driver uses to reach the test backend directly.
type TestService struct {
	Name         string
	UID          string
	Port         int
	InternalIP   string
	InternalPort int
	svcID        string
}

func (svc *TestService) Register(client *capi.Client, tags []string, meta map[string]string) error {
	if svc.svcID != "" {
		return nil
	}

	// Set ID explicitly so multiple instances can share a service Name
	// (consul agents default ID to Name, which would make the second
	// registration overwrite the first).
	err := client.Agent().ServiceRegister(&capi.AgentServiceRegistration{
		ID:      svc.UID,
		Name:    svc.Name,
		Address: svc.InternalIP,
		Port:    svc.InternalPort,
		Tags:    append(tags, svc.UID),
		Meta:    meta,
	})
	if err != nil {
		return err
	}

	// Resolve the catalog-assigned ServiceID by filtering on our unique UID
	// tag so we deregister exactly this instance later.
	matches, _, err := client.Catalog().Service(svc.Name, svc.UID, nil)
	if err != nil {
		return err
	}
	if len(matches) != 1 {
		return fmt.Errorf("unexpected response from catalog")
	}

	svc.svcID = matches[0].ServiceID
	return nil
}

func (svc *TestService) Unregister(client *capi.Client) error {
	if svc.svcID == "" {
		return nil
	}
	if err := client.Agent().ServiceDeregister(svc.svcID); err != nil {
		return err
	}
	svc.svcID = ""
	return nil
}

// Request/Response mirror the payloads accepted by the e2e `driver` command,
// which is the container that actually issues the request from a given
// network namespace.
type Request struct {
	URL    string `json:"url"`
	Host   string `json:"host"`
	Method string `json:"method"`
}

type Response struct {
	Status int    `json:"status"`
	Body   string `json:"body,omitempty"`
	Err    string `json:"error,omitempty"`
}

type Driver struct {
	Endpoint string
}

func (drv Driver) Request(url, host string) (int, string, error) {
	resp, err := callDriver(drv.Endpoint, Request{URL: url, Host: host})
	if err != nil {
		return 0, "", err
	}
	if resp.Err != "" {
		return 0, "", errors.New(resp.Err)
	}
	return resp.Status, resp.Body, nil
}

func callDriver(endpoint string, r Request) (*Response, error) {
	data, err := json.Marshal(&r)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rb Response
	if err := json.NewDecoder(resp.Body).Decode(&rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

// ExpectServiceRequest issues a request through drv and asserts the response
// code and body match the expected values. Must be called from inside a
// Ginkgo spec (or BeforeEach/AfterEach): it uses Gomega matchers.
func ExpectServiceRequest(drv Driver, url, host string, statusCode int, body string) {
	GinkgoHelper()
	rCode, rBody, err := drv.Request(url, host)
	Expect(err).NotTo(HaveOccurred(), "GET %s via %s", host, drv.Endpoint)
	Expect(rCode).To(Equal(statusCode), "GET %s via %s: status", host, drv.Endpoint)
	Expect(rBody).To(Equal(body), "GET %s via %s: body", host, drv.Endpoint)
}

// ExpectHostUnreachable asserts that the hostname is not published by the
// proxy. Caddy's internal TLS automation only provisions certs for
// configured hostnames, so an un-routed host fails the TLS handshake rather
// than returning a clean HTTP status — this helper tolerates either shape.
func ExpectHostUnreachable(drv Driver, url, host string) {
	GinkgoHelper()
	_, _, err := drv.Request(url, host)
	Expect(err).To(HaveOccurred(), "GET %s via %s: expected request to fail", host, drv.Endpoint)
}
