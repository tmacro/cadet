package e2e

import (
	"github.com/efficientgo/e2e"
	capi "github.com/hashicorp/consul/api"
)

func DeployConsul(e *e2e.DockerEnvironment, config []byte) (e2e.Runnable, error) {
	envVars := make(map[string]string)
	if config != nil {
		envVars["CONSUL_LOCAL_CONFIG"] = string(config)
	}

	c := e.Runnable("consul").
		WithPorts(
			map[string]int{
				"http": 8500,
				"dns":  8600,
			}).
		Init(e2e.StartOptions{
			Image:     "consul:1.15.4",
			Readiness: e2e.NewHTTPReadinessProbe("http", "/", 200, 200),
			EnvVars:   envVars,
		})

	if err := e2e.StartAndWaitReady(c); err != nil {
		return nil, err
	}

	return c, nil
}

func NewConsulClient(addr string) (*capi.Client, error) {
	cfg := capi.DefaultConfig()
	cfg.Address = addr

	client, err := capi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return client, nil
}
