package httpserver

import (
	"context"
	"net"
	"net/http"
	"time"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/tmacro/cadet/pkg/supervise"
)

func Server(addr string, handler http.Handler, middleware ...Middleware) supervise.Service {
	return supervise.ServiceFunc(func(ctx context.Context) error {
		for i := len(middleware) - 1; i >= 0; i-- {
			handler = middleware[i](handler)
		}
		srv := &http.Server{
			Addr:         addr,
			Handler:      handler,
			WriteTimeout: 30 * time.Second, // TODO: make configurable
			ReadTimeout:  30 * time.Second,
			BaseContext: func(listener net.Listener) context.Context {
				return ctx
			},
		}

		srvErr := make(chan error, 1)

		go func() {
			log.Trace().Msg("starting http server")

			err := srv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				srvErr <- err
			} else {
				srvErr <- nil
			}
			close(srvErr)
		}()

		select {
		case <-ctx.Done():
			log.Trace().Msg("shutting down http server")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdownCtx)
		case err := <-srvErr:
			log.Error().
				Err(err).
				Msg("error starting http server")
			return err
		}
	})
}
