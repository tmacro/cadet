package webui

import (
    "io/fs"
    "net/http"

    "github.com/gorilla/mux"

    "github.com/tmacro/cadet/pkg/log"
)

func Router(staticAssets fs.FS) http.Handler {
    r := mux.NewRouter()
    r.HandleFunc("/", handleIndex).Methods("GET")
    r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticAssets))))
    return r
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
    log.Debug("Handling index request")
    RenderAndRespond(w, r, "layout", nil, "layout.html", "index.html")
}
