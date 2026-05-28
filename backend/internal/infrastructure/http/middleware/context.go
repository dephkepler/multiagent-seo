package middleware

import (
	"context"

	"contentflow/pkg/logger"
)

func stringFromCtx(ctx context.Context, key logger.ContextKey) string {
	v, _ := ctx.Value(key).(string)
	return v
}
