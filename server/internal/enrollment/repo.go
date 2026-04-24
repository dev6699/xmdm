package enrollment

import (
	"context"
	"time"
)

type Repository interface {
	IssueToken(ctx context.Context, tenantID string, expiresAt time.Time) (IssuedToken, error)
	ValidateToken(ctx context.Context, tenantID, token string) (Token, error)
	ConsumeToken(ctx context.Context, tenantID, token string) (Token, error)
	RevokeToken(ctx context.Context, tenantID, id string) (Token, error)
	ExpireTokens(ctx context.Context, before time.Time) (int64, error)
}
