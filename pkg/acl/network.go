package acl

import (
	"encoding/json"
	"net"
	"strings"

	"github.com/rs/zerolog"
)

type Network []*net.IPNet

func (n Network) Matches(ip *net.IP) bool {
	if ip == nil {
		return false
	}

	for _, ntwk := range n {
		if ntwk.Contains(*ip) {
			return true
		}
	}

	return false
}

func (n Network) String() string {
	if n == nil {
		return "nil"
	}

	networks := make([]string, len(n))
	for i, ntwk := range n {
		networks[i] = ntwk.String()
	}

	return "[ " + strings.Join(networks, ", ") + " ]"
}

func (n *Network) UnmarshalJSON(data []byte) error {
	var raw []string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*n = make(Network, len(raw))
	for i, cidr := range raw {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return err
		}
		(*n)[i] = ipnet
	}

	return nil
}

func (n Network) MarshalZerologArray(a *zerolog.Array) {
	for _, ntwk := range n {
		a.Str(ntwk.String())
	}
}
