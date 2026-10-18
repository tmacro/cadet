package cadet

import (
	"fmt"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/tmacro/cadet/internal/webui"
)

func init() {
	caddy.RegisterModule(Dashboard{})
}

// Dashboard is an HTTP handler serving the cadet web UI. It reads services
// and global config from the cadet app, so that app must be configured.
type Dashboard struct {
	handler http.Handler
}

var (
	_ caddy.Module                = (*Dashboard)(nil)
	_ caddy.Provisioner           = (*Dashboard)(nil)
	_ caddyhttp.MiddlewareHandler = (*Dashboard)(nil)
)

func (Dashboard) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.cadet_dashboard",
		New: func() caddy.Module { return new(Dashboard) },
	}
}

func (d *Dashboard) Provision(ctx caddy.Context) error {
	app, err := ctx.AppIfConfigured("cadet")
	if err != nil {
		return fmt.Errorf("cadet dashboard requires the cadet app: %w", err)
	}

	plug, ok := app.(*Plugin)
	if !ok {
		return fmt.Errorf("unexpected cadet app type %T", app)
	}

	d.handler = webui.NewHandler(plug.watcher, webui.EmbeddedTemplates, webui.StaticAssets)
	return nil
}

func (d *Dashboard) ServeHTTP(w http.ResponseWriter, r *http.Request, _ caddyhttp.Handler) error {
	d.handler.ServeHTTP(w, r)
	return nil
}
