package acl

import (
	"encoding/json"
	"strings"

	"github.com/rs/zerolog"
)

type Policy []Control

func (p Policy) String() string {
	if len(p) == 0 {
		return ""
	}
	var str string
	for _, control := range p {
		str += control.String() + "; "
	}
	return str[:len(str)-1] // Remove trailing semicolon
}

func (p Policy) IsValid() bool {
	for _, control := range p {
		if !control.IsValid() {
			return false
		}
	}
	return true
}

func (p Policy) IsEmpty() bool {
	return len(p) == 0
}

func ParsePolicy(policyStr string) (Policy, error) {
	if policyStr == "" {
		return nil, nil
	}
	var policy Policy
	for part := range strings.SplitSeq(policyStr, ";") {
		control, err := ParseControl(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		policy = append(policy, *control)
	}
	return policy, nil
}

func MustParsePolicy(polictStr string) Policy {
	p, err := ParsePolicy(polictStr)
	if err != nil {
		panic(err)
	}

	return p
}

func (p *Policy) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte(`""`), nil
	}
	return []byte(`"` + p.String() + `"`), nil
}

func (p *Policy) UnmarshalJSON(data []byte) error {
	var policyStr string
	if err := json.Unmarshal(data, &policyStr); err != nil {
		return err
	}
	parsedPolicy, err := ParsePolicy(policyStr)
	if err != nil {
		return err
	}
	*p = parsedPolicy
	return nil
}

func (p Policy) Eval(entity string) (Action, bool, error) {
	if len(p) == 0 {
		return Deny, false, nil
	}

	for _, control := range p {
		if control.Matches(entity) {
			return control.Action, true, nil
		}
	}

	return Deny, false, nil
}

func (p Policy) MarshalZerologArray(a *zerolog.Array) {
	for _, control := range p {
		a.Object(&control)
	}
}
