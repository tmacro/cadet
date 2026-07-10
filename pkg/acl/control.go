package acl

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/rs/zerolog"
)

type Control struct {
	Action Action `json:"action"`
	Entity string `json:"entity"`
}

func (c *Control) String() string {
	return c.Action.String() + " " + c.Entity
}

func (c *Control) IsValid() bool {
	return c.Action.IsValid() && c.Entity != ""
}

func (c *Control) Matches(entity string) bool {
	return c.Entity == entity
}

func ParseControl(controlStr string) (*Control, error) {
	if controlStr == "" {
		return nil, errors.New("control string cannot be empty")
	}

	actionStr, entity, ok := strings.Cut(controlStr, " ")
	if !ok {
		return nil, errors.New("invalid control format, expected 'action entity'")
	}

	action, err := ParseAction(actionStr)
	if err != nil {
		return nil, err
	}

	return &Control{
		Action: action,
		Entity: entity,
	}, nil
}

func (c *Control) MarshalJSON() ([]byte, error) {
	return []byte(`"` + c.String() + `"`), nil
}

func (c *Control) UnmarshalJSON(data []byte) error {
	var controlStr string
	if err := json.Unmarshal(data, &controlStr); err != nil {
		return err
	}

	control, err := ParseControl(controlStr)
	if err != nil {
		return err
	}

	c.Action = control.Action
	c.Entity = control.Entity
	return nil
}

func (s *Control) MarshalZerologObject(e *zerolog.Event) {
	e.Str("action", s.Action.String()).Str("entity", s.Entity)
}
