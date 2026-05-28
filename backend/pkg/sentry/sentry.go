package sentry

import (
	"context"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"

	"contentflow/pkg/config"
	"contentflow/pkg/logger"
)

// BuildDate is injected at build time via -ldflags "-X contentflow/pkg/sentry.BuildDate=<value>".
var BuildDate string

var once sync.Once

type Sentry interface {
	Initialize() error
	Flush()
}

type sentryClient struct {
	cfg config.SentryConfig
}

func New(cfg config.SentryConfig) Sentry {
	return &sentryClient{cfg: cfg}
}

func (s *sentryClient) Initialize() error {
	log := logger.New(context.Background(), "sentry")

	var err error
	once.Do(func() {
		if !s.cfg.Enabled {
			log.Warn().Msg("Sentry DSN not set, initialization skipped")
			return
		}

		release := s.cfg.Release
		if release == "" {
			release = BuildDate
		}

		err = sentry.Init(sentry.ClientOptions{
			Dsn:              s.cfg.Dsn,
			Environment:      s.cfg.Environment,
			Release:          release,
			TracesSampleRate: 1.0,
		})

		if err != nil {
			log.Error().Err(err).Msg("Sentry initialization failed")
		} else {
			log.Debug().Msg("Sentry initialized")
		}
	})

	return err
}

// Flush waits for buffered events to be sent before shutdown.
func (s *sentryClient) Flush() {
	sentry.Flush(2 * time.Second)
}
