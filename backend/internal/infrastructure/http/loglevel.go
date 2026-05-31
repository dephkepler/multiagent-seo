package http

import (
	"encoding/json"
	"net/http"

	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/pkg/logger"
)

func handleGetLogLevel(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"level": logger.CurrentLevel()})
}

// handleSetLogLevel changes log verbosity at runtime (no restart). Body: {"level":"debug"}.
func handleSetLogLevel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		problem.Write(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := logger.SetLevel(body.Level); err != nil {
		problem.Write(w, http.StatusBadRequest, err.Error())
		return
	}
	l := logger.New(r.Context(), "admin")
	l.Info().Str("level", logger.CurrentLevel()).Msg("log level changed")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"level": logger.CurrentLevel()})
}
