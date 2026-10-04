package webui

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"

	sprig "github.com/go-task/slim-sprig"

	"github.com/tmacro/cadet/pkg/log"
)

type CacheStrategy int

const (
	NoCache CacheStrategy = iota
	Cache
)

type TemplateCache interface {
	Open(paths ...string) (*template.Template, error)
}

type cache struct {
	strategy CacheStrategy
	fs       fs.FS

	mu       sync.RWMutex
	compiled map[string]*template.Template
}

var _ TemplateCache = (*cache)(nil)

func (c *cache) Open(paths ...string) (*template.Template, error) {
	if c.strategy == NoCache {
		return c.compileTemplates(paths)
	}

	cacheKey := strings.Join(paths, "|")
	c.mu.RLock()
	t, ok := c.compiled[cacheKey]
	c.mu.RUnlock()
	if ok {
		return t, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok = c.compiled[cacheKey]
	if ok {
		return t, nil
	}

	t, err := c.compileTemplates(paths)
	if err != nil {
		return nil, fmt.Errorf("error compiling template: %w", err)
	}

	c.compiled[cacheKey] = t

	return t, nil
}

func (c *cache) compileTemplates(paths []string) (*template.Template, error) {
	t, err := template.New("").Funcs(sprig.FuncMap()).ParseFS(c.fs, paths...)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func NewTemplateCache(templates fs.FS, strategy CacheStrategy) TemplateCache {
	return &cache{
		strategy: strategy,
		fs:       templates,
		compiled: make(map[string]*template.Template),
	}
}

func TemplateCacheFromPath(path string, strategy CacheStrategy) TemplateCache {
	return NewTemplateCache(AssetsFromPath(path), strategy)
}

func RenderAndRespond(w http.ResponseWriter, r *http.Request, name string, data any, templates ...string) {
	tpl, err := TemplatesFromContext(r.Context(), templates...)
	if err != nil {
		log.Errorf("error getting templates: %s", err)
		http.Error(w, "error getting templates", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	err = tpl.ExecuteTemplate(&buf, name, data)
	if err != nil {
		log.Errorf("error rendering template: %s", err)
		http.Error(w, "error rendering template", http.StatusInternalServerError)
		return
	}

	body := buf.Bytes()
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
