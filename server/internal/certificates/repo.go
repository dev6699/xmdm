package certificates

import "context"

type Repository interface {
	ListCertificates(ctx context.Context, tenantID string) ([]Certificate, error)
	ListActiveCertificates(ctx context.Context, tenantID string) ([]Certificate, error)
	GetCertificate(ctx context.Context, tenantID, id string) (Certificate, error)
	CreateCertificate(ctx context.Context, tenantID string, req CertificateUpsert) (Certificate, error)
	RetireCertificate(ctx context.Context, tenantID, id string) (Certificate, error)
}
