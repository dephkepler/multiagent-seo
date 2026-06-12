package sentry

import (
	"context"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"

	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

var BuildDate string

var once sync.Once

type Client interface {
	Initialize() error
	Flush()
}

type sentryClient struct {
	cfg config.SentryConfig
}

func New(cfg config.SentryConfig) Client {
	return &sentryClient{cfg: cfg}
}

func (s *sentryClient) Initialize() error {
	log := logger.New(context.Background(), "sentry")

	var err error
	once.Do(func() {
		if !s.cfg.Enabled {
			log.Info().Bool("sentry_enabled", s.cfg.Enabled).Msg("sentry disabled, initialization skipped")
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
			log.Error().Err(err).
				Str("environment", s.cfg.Environment).
				Str("release", release).
				Msg("sentry initialization failed")
		} else {
			log.Debug().Msg("Sentry initialized")
		}
	})

	return err
}

func (s *sentryClient) Flush() {
	sentry.Flush(2 * time.Second)
}
