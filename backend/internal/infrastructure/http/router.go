package http

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	httpMiddleware "multiagent-seo/internal/infrastructure/http/middleware"
	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/internal/oapigen"
	"multiagent-seo/pkg/config"
	"multiagent-seo/pkg/logger"
)

func NewRouter(cfg config.ServerConfig, api oapigen.ServerInterface, authMW oapigen.MiddlewareFunc) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	// A wildcard origin with credentials is a credential-leak footgun (and the
	// browser rejects it anyway), so only allow credentials for explicit origins.
	allowCredentials := !slices.Contains(cfg.CORSAllowedOrigins, "*")
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: allowCredentials,
		MaxAge:           300,
	}))
	r.Use(httpMiddleware.SentryMiddleware())
	r.Use(httpMiddleware.RequestLogger)
	r.Use(httpMiddleware.SentryScopeEnhancer)

	sub := chi.NewRouter()
	var mws []oapigen.MiddlewareFunc
	if authMW != nil {
		mws = append(mws, authMW)
	}
	apiHandler := oapigen.HandlerWithOptions(api, oapigen.ChiServerOptions{
		BaseRouter:       sub,
		ErrorHandlerFunc: openAPIErrorHandler,
		Middlewares:      mws,
	})
	r.Mount(cfg.BasePath, apiHandler)
	return r
}

func openAPIErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	var rh *oapigen.RequiredHeaderError
	if errors.As(err, &rh) {
		problem.Write(w, http.StatusUnauthorized, fmt.Sprintf("missing required header: %s", rh.ParamName))
		return
	}
	log := logger.New(r.Context(), "openapi")
	log.Error().Err(err).Msg("strict server request error")
	problem.Write(w, http.StatusBadRequest, "invalid request")
}
