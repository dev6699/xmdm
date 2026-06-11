package certificates

import (
	"context"

	"xmdm/server/internal/pagination"
)

type Repository interface {
	ListCertificates(ctx context.Context, tenantID string, page pagination.Params) ([]Certificate, error)
	ListActiveCertificates(ctx context.Context, tenantID string, page pagination.Params) ([]Certificate, error)
	GetOverviewStats(ctx context.Context, tenantID string) (OverviewStats, error)
	GetCertificate(ctx context.Context, tenantID, id string) (Certificate, error)
	CreateCertificate(ctx context.Context, tenantID string, req CertificateUpsert) (Certificate, error)
	RetireCertificate(ctx context.Context, tenantID, id string) (Certificate, error)
}
