package appspg

import (
	"context"
	"errors"
	"time"

	"xmdm/server/internal/apps"
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

func (s *Store) CreateApp(ctx context.Context, tenantID string, req apps.AppUpsert) (apps.App, error) {
	if req.PackageName == "" || req.Name == "" {
		return apps.App{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO apps (id, tenant_id, package_name, name, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text, tenant_id::text, package_name, name, status, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.PackageName, req.Name, s.now(),
	)
	rec, err := scanApp(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apps.App{}, httpx.ErrConflict
		}
		return apps.App{}, err
	}
	return rec, nil
}

func (s *Store) ListApps(ctx context.Context, tenantID string) ([]apps.App, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, package_name, name, status, updated_at, deleted_at
		 FROM apps
		 WHERE tenant_id = $1
		 ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]apps.App, 0)
	for rows.Next() {
		rec, err := scanApp(rows)
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

func (s *Store) UpdateApp(ctx context.Context, tenantID, id string, req apps.AppUpsert) (apps.App, error) {
	if req.PackageName == "" || req.Name == "" {
		return apps.App{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE apps
		 SET package_name = $3, name = $4, updated_at = $5
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, package_name, name, status, updated_at, deleted_at`,
		tenantID, id, req.PackageName, req.Name, s.now(),
	)
	rec, err := scanApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apps.App{}, httpx.ErrNotFound
		}
		if isUniqueViolation(err) {
			return apps.App{}, httpx.ErrConflict
		}
		return apps.App{}, err
	}
	return rec, nil
}

func (s *Store) RetireApp(ctx context.Context, tenantID, id string) (apps.App, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE apps
		 SET status = $3, deleted_at = $4, updated_at = $4
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, package_name, name, status, updated_at, deleted_at`,
		tenantID, id, apps.StatusRetired, s.now(),
	)
	rec, err := scanApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apps.App{}, httpx.ErrNotFound
		}
		return apps.App{}, err
	}
	return rec, nil
}

func (s *Store) ListVersions(ctx context.Context, tenantID, appID string) ([]apps.Version, error) {
	var appExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM apps
		 WHERE tenant_id = $1 AND id = $2 AND status <> $3`,
		tenantID, appID, apps.StatusRetired,
	).Scan(&appExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, app_id::text, status, version_name, version_code, artifact_id, checksum, published_at, created_at
		 FROM app_versions
		 WHERE tenant_id = $1 AND app_id = $2
		 ORDER BY created_at`,
		tenantID, appID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]apps.Version, 0)
	for rows.Next() {
		rec, err := scanVersion(rows)
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

func (s *Store) CreateVersion(ctx context.Context, tenantID, appID string, req apps.VersionUpsert) (apps.Version, error) {
	if req.VersionName == "" || req.VersionCode <= 0 || req.Checksum == "" {
		return apps.Version{}, httpx.ErrInvalidInput
	}
	now := s.now()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apps.Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var appExists string
	if err := tx.QueryRow(ctx,
		`SELECT id::text
		 FROM apps
		 WHERE tenant_id = $1 AND id = $2 AND status <> $3`,
		tenantID, appID, apps.StatusRetired,
	).Scan(&appExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apps.Version{}, httpx.ErrNotFound
		}
		return apps.Version{}, err
	}

	status := apps.VersionStatusUploaded
	var publishedAt any
	if req.Publish {
		status = apps.VersionStatusPublished
		publishedAt = now
	}
	var artifactID any
	if req.ArtifactID != nil {
		artifactID = *req.ArtifactID
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO app_versions (id, tenant_id, app_id, version_name, version_code, artifact_id, checksum, status, published_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING id::text, tenant_id::text, app_id::text, status, version_name, version_code, artifact_id, checksum, published_at, created_at`,
		uuid.NewString(), tenantID, appID, req.VersionName, req.VersionCode, artifactID, req.Checksum, status, publishedAt, now,
	)
	rec, err := scanVersion(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apps.Version{}, httpx.ErrConflict
		}
		return apps.Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return apps.Version{}, err
	}
	return rec, nil
}

func scanApp(scanner rowScanner) (apps.App, error) {
	var rec apps.App
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.PackageName, &rec.Name, &rec.Status, &rec.UpdatedAt, &deletedAt); err != nil {
		return apps.App{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanVersion(scanner rowScanner) (apps.Version, error) {
	var rec apps.Version
	var artifactID pgtype.Text
	var publishedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.AppID, &rec.Status, &rec.VersionName, &rec.VersionCode, &artifactID, &rec.Checksum, &publishedAt, &rec.CreatedAt); err != nil {
		return apps.Version{}, err
	}
	if artifactID.Valid {
		value := artifactID.String
		rec.ArtifactID = &value
	}
	if publishedAt.Valid {
		rec.PublishedAt = &publishedAt.Time
	}
	return rec, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
