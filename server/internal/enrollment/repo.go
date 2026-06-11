package enrollment

import (
	"context"
	"time"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	IssueToken(ctx context.Context, tenantID string, expiresAt time.Time) (IssuedToken, error)
	ValidateToken(ctx context.Context, tenantID, token string) (Token, error)
	ConsumeToken(ctx context.Context, tenantID, token string) (Token, error)
	BindDevice(ctx context.Context, tenantID, token, deviceID string, bootstrapExtras map[string]any) (BoundDevice, error)
	ListTokens(ctx context.Context, tenantID string, page pagination.Params) ([]Token, error)
	RevokeToken(ctx context.Context, tenantID, id string) (Token, error)
	ExpireTokens(ctx context.Context, before time.Time) (int64, error)
}
