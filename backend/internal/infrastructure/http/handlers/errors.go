package handlers

import (
	"context"
	"errors"
	"net/http"

	"multiagent-seo/internal/infrastructure/http/problem"
	"multiagent-seo/pkg/logger"
)

type errMapping struct {
	err    error
	status int
	detail string // empty → falls back to err.Error()
	logMsg string // non-empty → logged at error level before responding
}

// E creates a mapping with a fixed detail string.
func E(err error, status int, detail string) errMapping {
	return errMapping{err: err, status: status, detail: detail}
}

// EMsg creates a mapping that uses err.Error() as the detail.
func EMsg(err error, status int) errMapping {
	return errMapping{err: err, status: status}
}

// ELog creates a mapping that logs at error level before writing the response.
func ELog(err error, status int, detail, logMsg string) errMapping {
	return errMapping{err: err, status: status, detail: detail, logMsg: logMsg}
}

type ErrMap struct {
	component string
	entries   []errMapping
}

func newErrMap(component string, entries ...errMapping) ErrMap {
	return ErrMap{component: component, entries: entries}
}

func (m ErrMap) Handle(ctx context.Context, w http.ResponseWriter, op string, err error) {
	for _, e := range m.entries {
		if errors.Is(err, e.err) {
			if e.logMsg != "" {
				log := logger.New(ctx, m.component)
				log.Error().Err(err).Str("op", op).Msg(e.logMsg)
			}
			detail := e.detail
			if detail == "" {
				detail = err.Error()
			}
			problem.Write(w, e.status, detail)
			return
		}
	}
	log := logger.New(ctx, m.component)
	log.Error().Err(err).Str("op", op).Msg("internal error")
	problem.Write(w, http.StatusInternalServerError, "internal error")
}
