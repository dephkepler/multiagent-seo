package problem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"multiagent-seo/pkg/logger"
)

const contentType = "application/problem+json"

type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	ext      map[string]any
}

func New(status int, detail string) *Problem {
	return &Problem{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	}
}

func (p *Problem) WithInstance(instance string) *Problem {
	p.Instance = instance
	return p
}

func (p *Problem) With(key string, value any) *Problem {
	switch key {
	case "type", "title", "status", "detail", "instance":
		panic(fmt.Sprintf("problem: extension key %q must not redefine a core RFC 9457 member", key))
	}
	if p.ext == nil {
		p.ext = make(map[string]any)
	}
	p.ext[key] = value
	return p
}

func (p *Problem) MarshalJSON() ([]byte, error) {
	type shadow Problem
	base, err := json.Marshal((*shadow)(p))
	if err != nil {
		return nil, err
	}
	if len(p.ext) == 0 {
		return base, nil
	}
	merged := make(map[string]any)
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range p.ext {
		merged[k] = v
	}
	return json.Marshal(merged)
}

func (p *Problem) WriteTo(w http.ResponseWriter) {
	// Marshal before committing the status so an encode failure can still fall
	// back to a minimal static body instead of a half-written response.
	log := logger.New(context.Background(), "problem")
	body, err := json.Marshal(p)
	if err != nil {
		log.Error().Err(err).Int("status", p.Status).Msg("marshal problem response failed")
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusInternalServerError)
		if _, werr := w.Write([]byte(`{"type":"about:blank","title":"Internal Server Error","status":500}`)); werr != nil {
			log.Error().Err(werr).Msg("write fallback problem response failed")
		}
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(p.Status)
	if _, err := w.Write(body); err != nil {
		log.Error().Err(err).Int("status", p.Status).Msg("write problem response failed")
	}
}

func Write(w http.ResponseWriter, status int, detail string) {
	New(status, detail).WriteTo(w)
}
