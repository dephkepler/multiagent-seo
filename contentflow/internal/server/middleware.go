package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// recoverMiddleware catches panics from downstream handlers, logs them
// with a stack trace, and returns a 500 to the client instead of
// letting the server connection be torn down.
func recoverMiddleware(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered",
					"err", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
