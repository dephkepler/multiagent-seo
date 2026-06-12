package response

import (
	"context"
	"encoding/json"
	"net/http"

	"multiagent-seo/pkg/logger"
)

func WriteJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	log := logger.New(ctx, "response")
	// Marshal before committing the status so a failure doesn't leave a body
	// half-written under an already-sent success header.
	b, err := json.Marshal(body)
	if err != nil {
		log.Error().Err(err).Int("status", status).Msg("marshal response failed")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, werr := w.Write([]byte(`{"error":"internal error"}`)); werr != nil {
			log.Error().Err(werr).Msg("write fallback response failed")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Int("status", status).Msg("write response failed")
	}
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
