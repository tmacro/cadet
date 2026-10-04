package webui

import (
    "fmt"
    "io/fs"
    "net/http"

    "github.com/gorilla/mux"

    "github.com/tmacro/cadet/pkg/log"
)

type fsSpy struct {
    fs fs.FS
}

func (spy *fsSpy) Open(name string) (fs.File, error) {
    fmt.Println("opening", name)
    return spy.fs.Open(name)
}

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
