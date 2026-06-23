package root

import (
	"context"

	appapitoken "multiagent-seo/internal/application/apitoken"
	domainauth "multiagent-seo/internal/domain/auth"
)

type compositeVerifier struct {
	jwt  domainauth.TokenVerifier
	keys *appapitoken.Service
}

func (c compositeVerifier) Verify(ctx context.Context, token string) (string, error) {
	if appapitoken.HasKeyPrefix(token) {
		uid, err := c.keys.Authenticate(ctx, token)
		if err != nil {
			return "", err
		}
		return uid.String(), nil
	}
	return c.jwt.Verify(ctx, token)
}
