package filespg

import (
	"context"
	"errors"
	"time"

	"xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/pagination"

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

func (s *Store) CreateFile(ctx context.Context, tenantID string, req files.FileUpsert) (files.File, error) {
	if req.Name == "" || req.StorageKey == "" || req.Checksum == "" || req.MimeType == "" || req.SizeBytes < 0 {
		return files.File{}, httpx.ErrInvalidInput
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return files.File{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	artifact, err := s.ensureArtifact(ctx, tx, tenantID, req)
	if err != nil {
		return files.File{}, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO files (id, tenant_id, name, artifact_id, checksum, mime_type, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id::text, tenant_id::text, name, status, updated_at, deleted_at, artifact_id::text, checksum, mime_type`,
		uuid.NewString(), tenantID, req.Name, artifact.ID, req.Checksum, req.MimeType, s.now(),
	)
	rec, err := scanFile(row)
	if err != nil {
		if isUniqueViolation(err) {
			return files.File{}, httpx.ErrConflict
		}
		return files.File{}, err
	}
	rec.Artifact = artifact
	if err := tx.Commit(ctx); err != nil {
		return files.File{}, err
	}
	return rec, nil
}

func (s *Store) ListFiles(ctx context.Context, tenantID string, page pagination.Params) ([]files.File, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT f.id::text, f.tenant_id::text, f.name, f.status, f.updated_at, f.deleted_at, f.artifact_id::text, f.checksum, f.mime_type,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM files f
		 JOIN artifacts a ON a.tenant_id = f.tenant_id AND a.id = f.artifact_id
		 WHERE f.tenant_id = $1
		 ORDER BY f.created_at, f.id
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]files.File, 0)
	for rows.Next() {
		rec, err := scanFileWithArtifact(rows)
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

func (s *Store) GetFile(ctx context.Context, tenantID, id string) (files.File, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT f.id::text, f.tenant_id::text, f.name, f.status, f.updated_at, f.deleted_at, f.artifact_id::text, f.checksum, f.mime_type,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM files f
		 JOIN artifacts a ON a.tenant_id = f.tenant_id AND a.id = f.artifact_id
		 WHERE f.tenant_id = $1 AND f.id = $2`,
		tenantID, id,
	)
	rec, err := scanFileWithArtifact(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.File{}, httpx.ErrNotFound
		}
		return files.File{}, err
	}
	return rec, nil
}

func (s *Store) RetireFile(ctx context.Context, tenantID, id string) (files.File, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE files
		 SET status = $3, deleted_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, status, updated_at, deleted_at, artifact_id::text, checksum, mime_type`,
		tenantID, id, files.StatusRetired, s.now(),
	)
	rec, err := scanFile(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.File{}, httpx.ErrNotFound
		}
		return files.File{}, err
	}
	artifact, err := s.loadArtifactByID(ctx, tenantID, rec.ArtifactID)
	if err == nil {
		rec.Artifact = artifact
	}
	return rec, nil
}

func (s *Store) ensureArtifact(ctx context.Context, tx pgx.Tx, tenantID string, req files.FileUpsert) (*files.Artifact, error) {
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

func (s *Store) ListOrphanArtifacts(ctx context.Context, tenantID string) ([]files.Artifact, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM artifacts a
		 WHERE a.tenant_id = $1
		   AND NOT EXISTS (
			   SELECT 1
			   FROM files f
			   WHERE f.tenant_id = a.tenant_id
			     AND f.artifact_id = a.id
		   )
		   AND NOT EXISTS (
			   SELECT 1
			   FROM certificates c
			   WHERE c.tenant_id = a.tenant_id
			     AND c.artifact_id = a.id
		   )
		   AND NOT EXISTS (
			   SELECT 1
			   FROM app_versions v
			   WHERE v.tenant_id = a.tenant_id
			     AND v.artifact_id = a.id::text
		   )
		 ORDER BY a.created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]files.Artifact, 0)
	for rows.Next() {
		rec, err := scanArtifact(rows)
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

func (s *Store) RetireArtifact(ctx context.Context, tenantID, id string) (files.Artifact, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE artifacts
		 SET status = $3, deleted_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, storage_key, checksum, size_bytes, mime_type, status, updated_at, deleted_at`,
		tenantID, id, files.StatusRetired, s.now(),
	)
	artifact, err := scanArtifact(row)
	if err != nil {
		return files.Artifact{}, err
	}
	return artifact, nil
}

func (s *Store) DeleteArtifact(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM artifacts
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	return err
}

func scanFile(scanner rowScanner) (files.File, error) {
	var rec files.File
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &rec.UpdatedAt, &deletedAt, &rec.ArtifactID, &rec.Checksum, &rec.MimeType); err != nil {
		return files.File{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanFileWithArtifact(scanner rowScanner) (files.File, error) {
	var rec files.File
	var deletedAt pgtype.Timestamptz
	var artifact files.Artifact
	var artifactDeletedAt pgtype.Timestamptz
	if err := scanner.Scan(
		&rec.ID, &rec.TenantID, &rec.Name, &rec.Status, &rec.UpdatedAt, &deletedAt, &rec.ArtifactID, &rec.Checksum, &rec.MimeType,
		&artifact.ID, &artifact.TenantID, &artifact.StorageKey, &artifact.Checksum, &artifact.SizeBytes, &artifact.MimeType, &artifact.Status, &artifact.UpdatedAt, &artifactDeletedAt,
	); err != nil {
		return files.File{}, err
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
