package webui

import (
	"io/fs"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/tmacro/cadet/pkg/config"
	"github.com/tmacro/cadet/pkg/log"
)

func Router(staticAssets fs.FS) http.Handler {
	r := mux.NewRouter()
	r.HandleFunc("/", handleServices).Methods("GET")
	r.HandleFunc("/config", handleConfig).Methods("GET")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticAssets))))
	return r
}

// NewHandler returns the dashboard handler with rc and templates attached to
// every request context, for hosts (like the caddy plugin) that don't own the
// server's base context.
func NewHandler(rc config.RemoteConfig, templates TemplateCache, staticAssets fs.FS) http.Handler {
	router := Router(staticAssets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ContextWithRemoteConfig(r.Context(), rc)
		ctx = ContextWithTemplates(ctx, templates)
		router.ServeHTTP(w, r.WithContext(ctx))
	})
}

// pageData is the root template context shared by every page; Nav names the
// active nav entry.
type pageData struct {
	Title string
	Nav   string
	Path  string
	Page  any
}

func remoteConfig(w http.ResponseWriter, r *http.Request) (config.RemoteConfig, bool) {
	rc, err := RemoteConfigFromContext(r.Context())
	if err != nil {
		log.Errorf("error getting remote config: %s", err)
		http.Error(w, "error getting remote config", http.StatusInternalServerError)
		return nil, false
	}
	return rc, true
}

func handleServices(w http.ResponseWriter, r *http.Request) {
	log.Debug("Handling services request")
	rc, ok := remoteConfig(w, r)
	if !ok {
		return
	}
	data := pageData{Title: "Services", Nav: "services", Path: r.URL.Path, Page: BuildServicesPage(rc)}
	RenderAndRespond(w, r, "layout", data, "layout.html", "services.html")
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	log.Debug("Handling config request")
	rc, ok := remoteConfig(w, r)
	if !ok {
		return
	}
	data := pageData{Title: "Config", Nav: "config", Path: r.URL.Path, Page: BuildConfigPage(rc)}
	RenderAndRespond(w, r, "layout", data, "layout.html", "config.html")
}
