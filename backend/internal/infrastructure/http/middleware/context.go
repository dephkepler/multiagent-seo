package middleware

import (
	"context"

	"multiagent-seo/pkg/logger"
)

func stringFromCtx(ctx context.Context, key logger.ContextKey) string {
	v, _ := ctx.Value(key).(string)
	return v
}
