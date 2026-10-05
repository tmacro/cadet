package supervise

import (
	"context"
	"github.com/rs/zerolog/log"
	"sync"

	"github.com/hashicorp/go-multierror"
)

type Service interface {
	Serve(context.Context) error
}

type ServiceFunc func(context.Context) error

func (sf ServiceFunc) Serve(ctx context.Context) error {
	return sf(ctx)
}

type Named interface {
	Name() string
}

func Serve(ctx context.Context, services ...Service) error {
	ctx, cancel := context.WithCancel(ctx)
	errors := make(chan error, len(services))

	var wg sync.WaitGroup
	wg.Add(len(services))

	for _, svc := range services {
		go runService(ctx, svc, errors, &wg)
	}

	var err error

	select {
	case <-ctx.Done():
	case err = <-errors:
	}

	cancel()

	go func() {
		wg.Wait()
		close(errors)
	}()

	for svcErr := range errors {
		err = multierror.Append(err, svcErr)
	}

	return err
}

func runService(ctx context.Context, svc Service, errors chan<- error, wg *sync.WaitGroup) {
	logger := log.Logger
	named, hasName := svc.(Named)
	if hasName {
		logger = logger.With().Str("service", named.Name()).Logger()
		logger.Debug().Msg("starting service")
	}

	if err := svc.Serve(ctx); err != nil {
		logger.Debug().
			Err(err).
			Msg("service exited with error")
		errors <- err
	} else if hasName {
		logger.Debug().
			Msg("service exited")
	}

	wg.Done()
}

func MustRun(ctx context.Context, svc Service) {
	err := Serve(ctx, svc)
	if err != nil {
		panic(err)
	}
}
