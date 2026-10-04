package webui

import (
	"context"
	"io/fs"
	"fmt"
	"html/template"

	"github.com/tmacro/cadet/pkg/config"
)

type contextKey string

const (
	templatesKey    contextKey = "templates"
	staticFilesKey             = "static-files"
	remoteConfigKey            = "remote-config"
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

func ContextWithStaticFiles(ctx context.Context, dir fs.FS) context.Context {
	return context.WithValue(ctx, staticFilesKey, dir)
}

func StaticFilesFromContext(ctx context.Context) (fs.FS, bool) {
	dir, ok := ctx.Value(staticFilesKey).(fs.FS)
	if !ok {
		return nil, ok
	}

	return dir, true
}

func ContextWithRemoteConfig(ctx context.Context, rc config.RemoteConfig) context.Context {
	return context.WithValue(ctx, remoteConfigKey, rc)
}

func RemoteConfigFromContext(ctx context.Context) (config.RemoteConfig, bool) {
	dir, ok := ctx.Value(remoteConfigKey).(config.RemoteConfig)
	if !ok {
		return nil, ok
	}

	return dir, true
}
