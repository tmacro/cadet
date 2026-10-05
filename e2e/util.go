package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/efficientgo/core/testutil"
	"github.com/efficientgo/e2e"
	"github.com/google/uuid"
	capi "github.com/hashicorp/consul/api"
	"github.com/jxskiss/base62"
)

func WriteFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = f.Write(data)
	if err != nil {
		return err
	}

	err = f.Sync()
	if err != nil {
		return err
	}

	return nil
}

func GetHostname(addr string) string {
	parts := strings.SplitN(addr, ":", 2)
	return parts[0]
}

func GetPort(addr string) int {
	parts := strings.SplitN(addr, ":", 2)
	p, err := strconv.Atoi(parts[1])
	if err != nil {
		panic(err)
	}

	return p
}

func ResolveEndpoint(ep string) (string, error) {
	hn := GetHostname(ep)
	addrs, err := net.LookupHost(hn)
	if err != nil {
		return "", err
	}

	return addrs[0], nil
}

type TestService struct {
	Name         string
	UID          string
	Port         int
	InternalIP   string
	InternalPort int
	svcID        string
}

func (svc *TestService) Endpoint() string {
	return fmt.Sprintf("127.0.0.1:%d", svc.Port)
}

func (svc *TestService) Register(client *capi.Client, tags []string, meta map[string]string) error {
	if svc.svcID != "" {
		return nil
	}

	err := client.Agent().ServiceRegister(&capi.AgentServiceRegistration{
		Name:    svc.Name,
		Address: svc.InternalIP,
		Port:    svc.InternalPort,
		Tags:    append(tags, svc.UID),
		Meta:    meta,
	})

	if err != nil {
		return err
	}

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
	err := client.Agent().ServiceDeregister(svc.svcID)
	if err != nil {
		return err
	}

	svc.svcID = ""
	return nil
}

func GenerateUUID() string {
	uuid, _ := uuid.New().MarshalBinary()
	id := base62.EncodeToString(uuid)
	// remove the padding
	return id[:len(id)-6]
}

type TestEnv struct {
	e2e.Environment

	Consul e2e.Runnable
	Caddy  e2e.Runnable

	ConsulClient *capi.Client
}

func (env *TestEnv) Stop() error {
	err := env.Caddy.Stop()
	if err != nil {
		return err
	}

	err = env.Consul.Stop()
	if err != nil {
		return err
	}

	env.Close()
	return nil
}

func (env *TestEnv) CaddyHTTPEndpoint() string {
	return "http://" + env.Caddy.Endpoint("http")
}

func (env *TestEnv) CaddyHTTPSEndpoint() string {
	return "https://" + env.Caddy.Endpoint("https")
}

// func DeployEnv() (*TestEnv, error) {
// 	e, err := e2e.New()
// 	if err != nil {
// 		return nil, err
// 	}

// 	defer func() {
// 		if err != nil {
// 			e.Close()
// 		}
// 	}()

// 	consul, err := DeployConsul(e, nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	defer func() {
// 		if err != nil {
// 			consul.Stop()
// 		}
// 	}()

// 	consulClient, err := NewConsulClient(consul)
// 	if err != nil {
// 		return nil, err
// 	}

// 	caddyCfg := FormatCaddyConfig("http://consul:8500")
// 	err = WriteCaddyConfig(consulClient, caddyCfg)
// 	if err != nil {
// 		return nil, err
// 	}

// 	err = WriteZoneConfig(consulClient)
// 	if err != nil {
// 		return nil, err
// 	}

// 	caddy, err := DeployCaddy(e, caddyCfg)
// 	if err != nil {
// 		return nil, err
// 	}

// 	env := TestEnv{
// 		Environment:  e,
// 		Consul:       consul,
// 		Caddy:        caddy,
// 		ConsulClient: consulClient,
// 	}

// 	return &env, nil
// }

func GetURL(rawUrl string) (int, []byte, error) {

	transport := &http.Transport{}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", rawUrl, nil)
	if err != nil {
		return 0, nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	return resp.StatusCode, body, nil
}

func CallDriver(endpoint string, r Request) (*Response, error) {
	data, err := json.Marshal(&r)
	if err != nil {
		return nil, err
	}

	body := bytes.NewReader(data)

	transport := &http.Transport{}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	req, err := http.NewRequest("POST", endpoint, body)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rb Response
	err = json.NewDecoder(resp.Body).Decode(&rb)
	if err != nil {
		return nil, err
	}

	return &rb, nil
}

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

func CheckService(t *testing.T, endpoint string, r Request, expected string) (bool, error) {
	resp, err := CallDriver(endpoint, r)
	if err != nil {
		return false, err
	}

	passed := true
	if resp.Status != 200 {
		t.Errorf("GET %s status = %d; want %d", r.URL, resp.Status, 200)
		passed = false
	}

	if resp.Body != expected {
		t.Errorf("GET %s body = \"%s\"; want \"%s\"", r.Host, resp.Body, expected)
		passed = false
	}

	return passed, nil
}

type Driver struct {
	Endpoint string
}

func (drv Driver) Request(url, host string) (int, string, error) {
	resp, err := CallDriver(drv.Endpoint, Request{
		URL:  url,
		Host: host,
	})

	if err != nil {
		return 0, "", err
	}

	if resp.Err != "" {
		return 0, "", errors.New(resp.Err)
	}

	return resp.Status, resp.Body, nil
}

func ExpectServiceRequest(t *testing.T, drv Driver, url, host string, statusCode int, body string) {
	rCode, rBody, err := drv.Request(url, host)
	testutil.Ok(t, err)

	if rCode != statusCode {
		t.Errorf("GET %s status = %d; want %d", host, rCode, statusCode)
	}

	if rBody != body {
		t.Errorf("GET %s body = \"%s\"; want \"%s\"", host, rBody, body)
	}
}
