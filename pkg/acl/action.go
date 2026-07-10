package acl

import (
	"encoding/json"
	"fmt"
)

type Action int

const (
	Allow Action = 1
	Deny  Action = 2
)

func (a Action) String() string {
	switch a {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	default:
		return "unknown"
	}
}

func (a Action) IsEmpty() bool {
	return a == 0
}

func (a Action) IsValid() bool {
	return a == Allow || a == Deny
}

func (a Action) IsAllow() bool {
	return a == Allow
}

func (a Action) IsDeny() bool {
	return a == Deny
}

func ParseAction(actionStr string) (Action, error) {
	switch actionStr {
	case "allow":
		return Allow, nil
	case "deny":
		return Deny, nil
	default:
		return Action(-1), fmt.Errorf("invalid action: %s", actionStr)
	}
}

func (a Action) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a *Action) UnmarshalJSON(data []byte) error {
	var actionStr string
	if err := json.Unmarshal(data, &actionStr); err != nil {
		return err
	}

	action, err := ParseAction(actionStr)
	if err != nil {
		return err
	}

	*a = action
	return nil
}
