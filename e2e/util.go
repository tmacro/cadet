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

// ResolveRequest/ResolveResponse mirror the payloads accepted by the
// driver's /resolve endpoint.
type ResolveRequest struct {
	Server string `json:"server"`
	Name   string `json:"name"`
}

type ResolveResponse struct {
	Rcode   string   `json:"rcode"`
	Answers []string `json:"answers,omitempty"`
	Err     string   `json:"error,omitempty"`
}

type Resolver struct {
	Endpoint string
}

// Resolve issues an A query for name against server from inside the
// resolver's network, returning the rcode and the A record addresses.
func (res Resolver) Resolve(server, name string) (string, []string, error) {
	var resp ResolveResponse
	if err := postJSON(res.Endpoint, ResolveRequest{Server: server, Name: name}, &resp); err != nil {
		return "", nil, err
	}
	if resp.Err != "" {
		return "", nil, errors.New(resp.Err)
	}
	return resp.Rcode, resp.Answers, nil
}

func callDriver(endpoint string, r Request) (*Response, error) {
	var rb Response
	if err := postJSON(endpoint, r, &rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

func postJSON(endpoint string, in, out any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return json.NewDecoder(resp.Body).Decode(out)
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

// ExpectRecords resolves name through res and asserts the answer contains
// exactly the given A record addresses, in any order.
func ExpectRecords(res Resolver, server, name string, addrs ...string) {
	GinkgoHelper()
	rcode, answers, err := res.Resolve(server, name)
	Expect(err).NotTo(HaveOccurred(), "resolve %s via %s", name, res.Endpoint)
	Expect(rcode).To(Equal("NOERROR"), "resolve %s via %s: rcode", name, res.Endpoint)
	Expect(answers).To(ConsistOf(addrs), "resolve %s via %s: answers", name, res.Endpoint)
}

// ExpectNoRecords asserts that name is not answered by the cadet plugin.
// Unanswered names fall through to the next plugin; with none configured
// that surfaces as SERVFAIL, so this only asserts on the empty answer.
func ExpectNoRecords(res Resolver, server, name string) {
	GinkgoHelper()
	_, answers, err := res.Resolve(server, name)
	Expect(err).NotTo(HaveOccurred(), "resolve %s via %s", name, res.Endpoint)
	Expect(answers).To(BeEmpty(), "resolve %s via %s: expected no answers", name, res.Endpoint)
}

// ExpectRefused asserts that name is denied by the ACL, which the cadet
// plugin answers with REFUSED rather than passing to the next plugin.
func ExpectRefused(res Resolver, server, name string) {
	GinkgoHelper()
	rcode, answers, err := res.Resolve(server, name)
	Expect(err).NotTo(HaveOccurred(), "resolve %s via %s", name, res.Endpoint)
	Expect(rcode).To(Equal("REFUSED"), "resolve %s via %s: rcode", name, res.Endpoint)
	Expect(answers).To(BeEmpty(), "resolve %s via %s: expected no answers", name, res.Endpoint)
}
