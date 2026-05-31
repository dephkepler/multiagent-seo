package http

import (
	"context"
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
	// Public, unauthenticated docs routes. Registered on the same base router as
	// the API operations; oapigen bakes auth into each operation handler (not via
	// router middleware), so these stay open.
	sub.Get("/", handleLandingPage)
	sub.Get("/docs", handleSwaggerUI)
	sub.Get("/docs/", handleSwaggerUI)
	sub.Get("/openapi.json", handleOpenAPISpec)

	// Runtime log-level control, gated behind bearer auth. oapigen normally sets
	// the auth marker per operation; for these hand-mounted routes we set it
	// ourselves so BearerAuth enforces a token.
	if authMW != nil {
		sub.Group(func(gr chi.Router) {
			gr.Use(requireBearer, authMW)
			gr.Get("/debug/log-level", handleGetLogLevel)
			gr.Put("/debug/log-level", handleSetLogLevel)
		})
	}

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

// requireBearer marks the request as auth-required so BearerAuth — which is
// otherwise gated on oapigen's per-operation scope marker — enforces a token.
func requireBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), oapigen.BearerAuthScopes, []string{})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
