package certificatesspg

import (
	"context"
	"errors"
	"time"

	"xmdm/server/internal/certificates"
	files "xmdm/server/internal/files"
	"xmdm/server/internal/httpx"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type rowScanner interface {
	Scan(...any) error
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, now: time.Now}
}

func (s *Store) SetNow(now func() time.Time) {
	s.now = now
}

func (s *Store) CreateCertificate(ctx context.Context, tenantID string, req certificates.CertificateUpsert) (certificates.Certificate, error) {
	if req.Name == "" || req.StorageKey == "" || req.Checksum == "" || req.MimeType == "" || req.SizeBytes < 0 {
		return certificates.Certificate{}, httpx.ErrInvalidInput
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return certificates.Certificate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	artifact, err := s.ensureArtifact(ctx, tx, tenantID, req)
	if err != nil {
		return certificates.Certificate{}, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO certificates (id, tenant_id, name, artifact_id, checksum, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text, tenant_id::text, name, status, updated_at, deleted_at, artifact_id::text, checksum`,
		uuid.NewString(), tenantID, req.Name, artifact.ID, req.Checksum, s.now(),
	)
	rec, err := scanCertificate(row)
	if err != nil {
		if isUniqueViolation(err) {
			return certificates.Certificate{}, httpx.ErrConflict
		}
		return certificates.Certificate{}, err
	}
	rec.Artifact = artifact
	if err := tx.Commit(ctx); err != nil {
		return certificates.Certificate{}, err
	}
	return rec, nil
}

func (s *Store) ListCertificates(ctx context.Context, tenantID string) ([]certificates.Certificate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id::text, c.tenant_id::text, c.name, c.status, c.updated_at, c.deleted_at, c.artifact_id::text, c.checksum,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM certificates c
		 JOIN artifacts a ON a.tenant_id = c.tenant_id AND a.id = c.artifact_id
		 WHERE c.tenant_id = $1
		 ORDER BY c.created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]certificates.Certificate, 0)
	for rows.Next() {
		rec, err := scanCertificateWithArtifact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) ListActiveCertificates(ctx context.Context, tenantID string) ([]certificates.Certificate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id::text, c.tenant_id::text, c.name, c.status, c.updated_at, c.deleted_at, c.artifact_id::text, c.checksum,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM certificates c
		 JOIN artifacts a ON a.tenant_id = c.tenant_id AND a.id = c.artifact_id
		 WHERE c.tenant_id = $1 AND c.status = $2
		 ORDER BY c.created_at`,
		tenantID, certificates.StatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]certificates.Certificate, 0)
	for rows.Next() {
		rec, err := scanCertificateWithArtifact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) RetireCertificate(ctx context.Context, tenantID, id string) (certificates.Certificate, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE certificates
		 SET status = $3, deleted_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, status, updated_at, deleted_at, artifact_id::text, checksum`,
		tenantID, id, certificates.StatusRetired, s.now(),
	)
	rec, err := scanCertificate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return certificates.Certificate{}, httpx.ErrNotFound
		}
		return certificates.Certificate{}, err
	}
	artifact, err := s.loadArtifactByID(ctx, tenantID, rec.ArtifactID)
	if err == nil {
		rec.Artifact = artifact
	}
	return rec, nil
}

func (s *Store) ensureArtifact(ctx context.Context, tx pgx.Tx, tenantID string, req certificates.CertificateUpsert) (*files.Artifact, error) {
	row := tx.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, storage_key, checksum, size_bytes, mime_type, status, updated_at, deleted_at
		 FROM artifacts
		 WHERE tenant_id = $1 AND storage_key = $2`,
		tenantID, req.StorageKey,
	)
	artifact, err := scanArtifact(row)
	if err == nil {
		if artifact.Checksum != req.Checksum || artifact.SizeBytes != req.SizeBytes || artifact.MimeType != req.MimeType {
			return nil, httpx.ErrConflict
		}
		return &artifact, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	row = tx.QueryRow(ctx,
		`INSERT INTO artifacts (id, tenant_id, storage_key, checksum, size_bytes, mime_type, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id::text, tenant_id::text, storage_key, checksum, size_bytes, mime_type, status, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.StorageKey, req.Checksum, req.SizeBytes, req.MimeType, s.now(),
	)
	artifact, err = scanArtifact(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, httpx.ErrConflict
		}
		return nil, err
	}
	return &artifact, nil
}

func (s *Store) loadArtifactByID(ctx context.Context, tenantID, artifactID string) (*files.Artifact, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, storage_key, checksum, size_bytes, mime_type, status, updated_at, deleted_at
		 FROM artifacts
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, artifactID,
	)
	artifact, err := scanArtifact(row)
	if err != nil {
		return nil, err
	}
	return &artifact, nil
}

func scanCertificate(scanner rowScanner) (certificates.Certificate, error) {
	var rec certificates.Certificate
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &rec.UpdatedAt, &deletedAt, &rec.ArtifactID, &rec.Checksum); err != nil {
		return certificates.Certificate{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanCertificateWithArtifact(scanner rowScanner) (certificates.Certificate, error) {
	var rec certificates.Certificate
	var deletedAt pgtype.Timestamptz
	var artifact files.Artifact
	var artifactDeletedAt pgtype.Timestamptz
	if err := scanner.Scan(
		&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &rec.UpdatedAt, &deletedAt, &rec.ArtifactID, &rec.Checksum,
		&artifact.ID, &artifact.TenantID, &artifact.StorageKey, &artifact.Checksum, &artifact.SizeBytes, &artifact.MimeType, &artifact.Status, &artifact.UpdatedAt, &artifactDeletedAt,
	); err != nil {
		return certificates.Certificate{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if artifactDeletedAt.Valid {
		artifact.DeletedAt = &artifactDeletedAt.Time
	}
	rec.Artifact = &artifact
	return rec, nil
}

func scanArtifact(scanner rowScanner) (files.Artifact, error) {
	var rec files.Artifact
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.StorageKey, &rec.Checksum, &rec.SizeBytes, &rec.MimeType, &rec.Status, &rec.UpdatedAt, &deletedAt); err != nil {
		return files.Artifact{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
