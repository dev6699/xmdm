package managedfilespg

import (
	"context"
	"errors"
	"strings"
	"time"

	files "xmdm/server/internal/files"
	"xmdm/server/internal/httpx"
	"xmdm/server/internal/managedfiles"

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

func (s *Store) CreateManagedFile(ctx context.Context, tenantID string, req managedfiles.ManagedFileUpsert) (managedfiles.ManagedFile, error) {
	if req.FileID == "" || strings.TrimSpace(req.Path) == "" {
		return managedfiles.ManagedFile{}, httpx.ErrInvalidInput
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return managedfiles.ManagedFile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	source, err := s.loadSourceFile(ctx, tx, tenantID, req.FileID)
	if err != nil {
		return managedfiles.ManagedFile{}, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO managed_files (id, tenant_id, file_id, path, replace_variables, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id::text, tenant_id::text, file_id::text, path, status, updated_at, deleted_at, replace_variables`,
		uuid.NewString(), tenantID, req.FileID, strings.TrimSpace(req.Path), req.ReplaceVariables, s.now(),
	)
	rec, err := scanManagedFile(row)
	if err != nil {
		if isUniqueViolation(err) {
			return managedfiles.ManagedFile{}, httpx.ErrConflict
		}
		return managedfiles.ManagedFile{}, err
	}
	rec.File = source
	if err := tx.Commit(ctx); err != nil {
		return managedfiles.ManagedFile{}, err
	}
	return rec, nil
}

func (s *Store) ListManagedFiles(ctx context.Context, tenantID string) ([]managedfiles.ManagedFile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id::text, m.tenant_id::text, m.file_id::text, m.path, m.status, m.updated_at, m.deleted_at, m.replace_variables,
		        f.id::text, f.tenant_id::text, f.name, f.status, f.updated_at, f.deleted_at, f.artifact_id::text, f.checksum, f.mime_type,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM managed_files m
		 JOIN files f ON f.tenant_id = m.tenant_id AND f.id = m.file_id
		 JOIN artifacts a ON a.tenant_id = f.tenant_id AND a.id = f.artifact_id
		 WHERE m.tenant_id = $1
		 ORDER BY m.created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]managedfiles.ManagedFile, 0)
	for rows.Next() {
		rec, err := scanManagedFileWithSource(rows)
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

func (s *Store) GetManagedFile(ctx context.Context, tenantID, id string) (managedfiles.ManagedFile, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT m.id::text, m.tenant_id::text, m.file_id::text, m.path, m.status, m.updated_at, m.deleted_at, m.replace_variables,
		        f.id::text, f.tenant_id::text, f.name, f.status, f.updated_at, f.deleted_at, f.artifact_id::text, f.checksum, f.mime_type,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM managed_files m
		 JOIN files f ON f.tenant_id = m.tenant_id AND f.id = m.file_id
		 JOIN artifacts a ON a.tenant_id = f.tenant_id AND a.id = f.artifact_id
		 WHERE m.tenant_id = $1 AND m.id = $2`,
		tenantID, id,
	)
	rec, err := scanManagedFileWithSource(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return managedfiles.ManagedFile{}, httpx.ErrNotFound
		}
		return managedfiles.ManagedFile{}, err
	}
	return rec, nil
}

func (s *Store) RetireManagedFile(ctx context.Context, tenantID, id string) (managedfiles.ManagedFile, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE managed_files
		 SET status = $3, deleted_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, file_id::text, path, status, updated_at, deleted_at, replace_variables`,
		tenantID, id, managedfiles.StatusRetired, s.now(),
	)
	rec, err := scanManagedFile(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return managedfiles.ManagedFile{}, httpx.ErrNotFound
		}
		return managedfiles.ManagedFile{}, err
	}
	source, err := s.loadSourceFile(ctx, s.pool, tenantID, rec.FileID)
	if err == nil {
		rec.File = source
	}
	return rec, nil
}

func (s *Store) loadSourceFile(ctx context.Context, q queryer, tenantID, fileID string) (*files.File, error) {
	row := q.QueryRow(ctx,
		`SELECT f.id::text, f.tenant_id::text, f.name, f.status, f.updated_at, f.deleted_at, f.artifact_id::text, f.checksum, f.mime_type,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM files f
		 JOIN artifacts a ON a.tenant_id = f.tenant_id AND a.id = f.artifact_id
		 WHERE f.tenant_id = $1 AND f.id = $2`,
		tenantID, fileID,
	)
	rec, err := scanFileWithArtifact(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	if rec.Status != files.StatusActive {
		return nil, httpx.ErrNotFound
	}
	return &rec, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func scanManagedFile(scanner rowScanner) (managedfiles.ManagedFile, error) {
	var rec managedfiles.ManagedFile
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.FileID, &rec.Path, &rec.Status, &rec.UpdatedAt, &deletedAt, &rec.ReplaceVariables); err != nil {
		return managedfiles.ManagedFile{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanManagedFileWithSource(scanner rowScanner) (managedfiles.ManagedFile, error) {
	var rec managedfiles.ManagedFile
	var deletedAt pgtype.Timestamptz
	var source files.File
	var sourceDeletedAt pgtype.Timestamptz
	var artifact files.Artifact
	var artifactDeletedAt pgtype.Timestamptz
	if err := scanner.Scan(
		&rec.ID, &rec.TenantID, &rec.FileID, &rec.Path, &rec.Status, &rec.UpdatedAt, &deletedAt, &rec.ReplaceVariables,
		&source.ID, &source.TenantID, &source.Name, &source.Status, &source.UpdatedAt, &sourceDeletedAt, &source.ArtifactID, &source.Checksum, &source.MimeType,
		&artifact.ID, &artifact.TenantID, &artifact.StorageKey, &artifact.Checksum, &artifact.SizeBytes, &artifact.MimeType, &artifact.Status, &artifact.UpdatedAt, &artifactDeletedAt,
	); err != nil {
		return managedfiles.ManagedFile{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if sourceDeletedAt.Valid {
		source.DeletedAt = &sourceDeletedAt.Time
	}
	if artifactDeletedAt.Valid {
		artifact.DeletedAt = &artifactDeletedAt.Time
	}
	source.Artifact = &artifact
	rec.File = &source
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

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
