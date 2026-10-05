package webui

import (
	"context"
	"fmt"
	"html/template"

	"github.com/tmacro/cadet/pkg/config"
)

type contextKey string

const (
	templatesKey    contextKey = "templates"
	remoteConfigKey contextKey = "remote-config"
)

func ContextWithTemplates(ctx context.Context, t TemplateCache) context.Context {
	return context.WithValue(ctx, templatesKey, t)
}

func TemplatesFromContext(ctx context.Context, name ...string) (*template.Template, error) {
	t, ok := ctx.Value(templatesKey).(TemplateCache)
	if !ok {
		return nil, fmt.Errorf("templates not found in context")
	}
	return t.Open(name...)
}

func ContextWithRemoteConfig(ctx context.Context, rc config.RemoteConfig) context.Context {
	return context.WithValue(ctx, remoteConfigKey, rc)
}
