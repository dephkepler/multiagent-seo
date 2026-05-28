package response

import (
	"context"
	"encoding/json"
	"net/http"

	"multiagent-seo/pkg/logger"
)

func WriteJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log := logger.New(ctx, "response")
		log.Error().Err(err).Msg("encode response failed")
	}
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
