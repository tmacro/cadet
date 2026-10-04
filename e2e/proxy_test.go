package e2e

import (
	"testing"
	"time"

	"github.com/efficientgo/core/testutil"
)

// func deployConsul(e *e2e.DockerEnvironment) (e2e.Runnable, error) {

// 	_, err = client.KV().Put(&capi.KVPair{
// 		Key: "cadet/acl.json",
// 		Value: fmt.Append([]byte(`
// 			{
// 			    "networks" : {
// 			        "local": ["127.0.0.0/8"],
// 			        "admin": ["10.10.0.0/24"],
// 			        "users": ["10.20.0.0/24"],
// 			        "lab": ["10.40.0.0/24"]
// 			    },
// 			    "groups" : {
// 			        "all": ["local", "admin", "users", "lab"]
// 			    },
// 			    "default_policy": "allow local",
// 			    "default_action": "deny"
// 			}`)),
// 	}, nil)
// 	if err != nil {
// 		return nil, err
// 	}

// 	_, err = client.KV().Put(&capi.KVPair{
// 		Key: "cadet/zone.json",
// 		Value: fmt.Append([]byte(`
// 			{
// 			    "default_zone": "tmacs.cloud",
// 			    "zones": ["tmacs.cloud", "binha.us"]
// 			}`)),
// 	}, nil)
// 	if err != nil {
// 		return nil, err
// 	}

//		return c, nil
//	}
//

var svc1 = &TestService{
	Name:         "svc1",
	UID:          GenerateUUID(),
	Port:         18080,
	InternalPort: 8080,
	InternalIP:   "172.30.0.4",
}

var svc2 = &TestService{
	Name:         "svc2",
	UID:          GenerateUUID(),
	Port:         18081,
	InternalPort: 8080,
	InternalIP:   "172.30.0.5",
}

var driver1 = Driver{
	Endpoint: "http://127.0.0.1:28080/request",
}

var driver2 = Driver{
	Endpoint: "http://127.0.0.1:28081/request",
}

func TestPrimaryZone(t *testing.T) {
	consulClient, err := NewConsulClient("http://127.0.0.1:8500")
	testutil.Ok(t, err)

	err = WriteCaddyConfig(consulClient, FormatCaddyConfig("http://consul:8500"))
	testutil.Ok(t, err)

	err = WriteZoneConfig(consulClient)
	testutil.Ok(t, err)

	svc1.Register(consulClient, []string{"cadet"}, nil)
	defer svc1.Unregister(consulClient)

	svc2.Register(consulClient, []string{"cadet"}, nil)
	defer svc2.Unregister(consulClient)

	time.Sleep(5 * time.Second)

	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")

	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
}

func TestSecondaryZone(t *testing.T) {
	consulClient, err := NewConsulClient("http://127.0.0.1:8500")
	testutil.Ok(t, err)

	err = WriteCaddyConfig(consulClient, FormatCaddyConfig("http://consul:8500"))
	testutil.Ok(t, err)

	err = WriteZoneConfig(consulClient)
	testutil.Ok(t, err)

	err = svc1.Register(consulClient, []string{"cadet"}, map[string]string{"cadet-zone": "alternate.internal"})
	testutil.Ok(t, err)
	defer svc1.Unregister(consulClient)

	err = svc2.Register(consulClient, []string{"cadet"}, map[string]string{"cadet-zone": "alternate.internal"})
	testutil.Ok(t, err)
	defer svc2.Unregister(consulClient)

	time.Sleep(5 * time.Second)

	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc1.alternate.internal", 200, "svc1")
	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc2.alternate.internal", 200, "svc2")

	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc1.alternate.internal", 200, "svc1")
	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc2.alternate.internal", 200, "svc2")
}

func TestACLDefaultDeny(t *testing.T) {
	consulClient, err := NewConsulClient("http://127.0.0.1:8500")
	testutil.Ok(t, err)

	err = WriteCaddyConfig(consulClient, FormatCaddyConfig("http://consul:8500"))
	testutil.Ok(t, err)

	err = WriteZoneConfig(consulClient)
	testutil.Ok(t, err)

	err = WriteACLConfig(consulClient, []byte(`
		{
		    "networks" : {
		        "neta": ["172.30.1.0/24"],
		        "netb": ["172.30.2.0/24"]
		    },
		    "groups" : {
		        "all": ["neta", "netb"]
		    },
		    "default_policy": "allow neta",
		    "default_action": "deny"
		}`))
	testutil.Ok(t, err)
	defer DeleteACLConfig(consulClient)

	svc1.Register(consulClient, []string{"cadet"}, nil)
	defer svc1.Unregister(consulClient)

	svc2.Register(consulClient, []string{"cadet"}, nil)
	defer svc2.Unregister(consulClient)

	time.Sleep(5 * time.Second)

	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")

	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc1.cadet.internal", 404, "Not Found")
	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")
}

func TestACLDefaultAllow(t *testing.T) {
	consulClient, err := NewConsulClient("http://127.0.0.1:8500")
	testutil.Ok(t, err)

	err = WriteCaddyConfig(consulClient, FormatCaddyConfig("http://consul:8500"))
	testutil.Ok(t, err)

	err = WriteZoneConfig(consulClient)
	testutil.Ok(t, err)

	err = WriteACLConfig(consulClient, []byte(`
		{
		    "networks" : {
		        "neta": ["172.30.1.0/24"],
		        "netb": ["172.30.2.0/24"]
		    },
		    "groups" : {
		        "all": ["neta", "netb"]
		    },
		    "default_policy": "deny neta",
		    "default_action": "allow"
		}`))
	testutil.Ok(t, err)
	defer DeleteACLConfig(consulClient)

	svc1.Register(consulClient, []string{"cadet"}, nil)
	defer svc1.Unregister(consulClient)

	svc2.Register(consulClient, []string{"cadet"}, nil)
	defer svc2.Unregister(consulClient)

	time.Sleep(5 * time.Second)

	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc1.cadet.internal", 404, "Not Found")
	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")

	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
}

func TestACLServiceOverride(t *testing.T) {
	consulClient, err := NewConsulClient("http://127.0.0.1:8500")
	testutil.Ok(t, err)

	err = WriteCaddyConfig(consulClient, FormatCaddyConfig("http://consul:8500"))
	testutil.Ok(t, err)

	err = WriteZoneConfig(consulClient)
	testutil.Ok(t, err)

	err = WriteACLConfig(consulClient, []byte(`
		{
		    "networks" : {
		        "neta": ["172.30.1.0/24"],
		        "netb": ["172.30.2.0/24"]
		    },
		    "groups" : {
		        "all": ["neta", "netb"]
		    },
		    "default_policy": "deny all",
		    "default_action": "deny"
		}`))
	testutil.Ok(t, err)
	defer DeleteACLConfig(consulClient)

	svc1.Register(consulClient, []string{"cadet"}, map[string]string{"cadet-acl": "allow neta"})
	defer svc1.Unregister(consulClient)

	svc2.Register(consulClient, []string{"cadet"}, map[string]string{"cadet-acl": "allow netb"})
	defer svc2.Unregister(consulClient)

	time.Sleep(5 * time.Second)

	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")

	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc1.cadet.internal", 404, "Not Found")
	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
}

func TestACLGroup(t *testing.T) {
	consulClient, err := NewConsulClient("http://127.0.0.1:8500")
	testutil.Ok(t, err)

	err = WriteCaddyConfig(consulClient, FormatCaddyConfig("http://consul:8500"))
	testutil.Ok(t, err)

	err = WriteZoneConfig(consulClient)
	testutil.Ok(t, err)

	err = WriteACLConfig(consulClient, []byte(`
		{
		    "networks" : {
		        "neta": ["172.30.1.0/24"],
		        "netb": ["172.30.2.0/24"]
		    },
		    "groups" : {
		        "all": ["neta", "netb"]
		    },
		    "default_policy": "deny all",
		    "default_action": "deny"
		}`))
	testutil.Ok(t, err)
	defer DeleteACLConfig(consulClient)

	svc1.Register(consulClient, []string{"cadet"}, map[string]string{"cadet-acl": "allow all"})
	defer svc1.Unregister(consulClient)

	svc2.Register(consulClient, []string{"cadet"}, map[string]string{"cadet-acl": "allow netb"})
	defer svc2.Unregister(consulClient)

	time.Sleep(5 * time.Second)

	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver1, "https://caddy:443", "svc2.cadet.internal", 404, "Not Found")

	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc1.cadet.internal", 200, "svc1")
	ExpectServiceRequest(t, driver2, "https://caddy:443", "svc2.cadet.internal", 200, "svc2")
}
