package http

import (
	"context"
	"encoding/json"
	"net/http"

	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/pkg/logger"
)

func writeLevelJSON(ctx context.Context, w http.ResponseWriter) {
	log := logger.New(ctx, "admin")
	b, err := json.Marshal(map[string]string{"level": logger.CurrentLevel()})
	if err != nil {
		problem.Write(w, http.StatusInternalServerError, "internal error")
		log.Error().Err(err).Msg("marshal log level response failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Msg("write log level response failed")
	}
}

func handleGetLogLevel(w http.ResponseWriter, r *http.Request) {
	writeLevelJSON(r.Context(), w)
}

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
	writeLevelJSON(r.Context(), w)
}
