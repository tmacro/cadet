package httpserver

import (
	"time"
	"net/http"

	//	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/hlog"

	"github.com/tmacro/cadet/pkg/log"
)

type Middleware func(handler http.Handler) http.Handler

func accessHandler(r *http.Request, status, size int, duration time.Duration) {
	//	log.Info("got request", "method", r.Method, "url", r.URL, "size", strconv.FormatInt(int64(size), 10), "duration", duration.String())
	log.Infof("got request: method=%s url=%s status=%d, size=%d duration=%s", r.Method, r.URL, status, size, duration)
	//		Str("method", r.Method).
	//		Stringer("url", r.URL).
	//		Int("status", status).
	//		Int("size", size).
	//		Dur("duration", duration).
	//		Msg("got request")
}

func RequestLogger(next http.Handler) http.Handler {
	return hlog.AccessHandler(accessHandler)(next)
}
