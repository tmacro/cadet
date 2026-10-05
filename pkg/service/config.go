package service

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"

	"github.com/rs/zerolog"
	
	"github.com/tmacro/cadet/pkg/acl"
)

type ServiceType string

const (
	HTTP ServiceType = "http"
	TCP  ServiceType = "tcp"
	UDP  ServiceType = "udp"
)

func (s ServiceType) String() string {
	return string(s)
}

func (s ServiceType) IsValid() bool {
	switch s {
	case HTTP, TCP, UDP:
		return true
	default:
		return false
	}
}

func (s ServiceType) IsEmpty() bool {
	return s == ServiceType("")
}

type Endpoint struct {
	Address net.IP
	Port    uint16
}

func (e Endpoint) String() string {
	return net.JoinHostPort(e.Address.String(), strconv.FormatUint(uint64(e.Port), 10))
}

func (e *Endpoint) UnmarshalJSON(data []byte) error {
	var raw struct {
		Address string `json:"address"`
		Port    uint16 `json:"port"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if raw.Address != "" {
		e.Address = net.ParseIP(raw.Address)
		if e.Address == nil {
			return errors.New("invalid IP address")
		}
	}

	e.Port = raw.Port

	return nil
}

func (e Endpoint) MarshalZerologObject(u *zerolog.Event) {
	u.Str("address", e.Address.String())
	u.Uint16("port", e.Port)
}

type EndpointList []Endpoint

func (el EndpointList) MarshalZerologArray(a *zerolog.Array) {
	for _, s := range el {
		a.Object(s)
	}
}

type Config struct {
	ID        string       `json:"id"`
	Type      ServiceType  `json:"type"`
	Name      string       `json:"name"`
	Endpoints EndpointList `json:"endpoints"`
	Zones     []string     `json:"zones"`
	ACLs      acl.Policy   `json:"acls"`
	NoPublish bool         `json:"no_publish"`
	UseProxy  bool         `json:"use_proxy"`
}

func (c *Config) MarshalZerologObject(u *zerolog.Event) {
	u.Str("type", c.Type.String())
	u.Str("name", c.Name)
	u.Bool("no_publish", c.NoPublish)
	u.Bool("use_proxy", c.UseProxy)
	u.Array("endpoints", &c.Endpoints)
	u.Strs("zones", c.Zones)
}

func (c *Config) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID        string       `json:"id"`
		Type      string       `json:"type"`
		Name      string       `json:"name"`
		Endpoints EndpointList `json:"endpoints"`
		Zones     []string     `json:"zones"`
		NoPublish *bool        `json:"no_publish"`
		UseProxy  *bool        `json:"use_proxy"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.ID = raw.ID
	c.Type = ServiceType(raw.Type)
	c.Name = raw.Name
	c.Endpoints = raw.Endpoints
	c.Zones = raw.Zones

	if raw.NoPublish != nil {
		c.NoPublish = *raw.NoPublish
	} else {
		c.NoPublish = false
	}

	if raw.UseProxy != nil {
		c.UseProxy = *raw.UseProxy
	} else {
		c.UseProxy = true
	}

	return nil
}


type Map map[string]*Config

func CombineServiceMaps(svcMaps []Map) Map {
	combined := make(Map)

	for _, svcMap := range svcMaps {
		for key, svc := range svcMap {
			if _, ok := combined[key]; ok {
				continue
			}
			combined[key] = svc
		}
	}

	return combined
}
