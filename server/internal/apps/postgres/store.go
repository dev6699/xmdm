package appspg

import (
	"context"
	"errors"
	"time"

	"xmdm/server/internal/apps"
	files "xmdm/server/internal/files"
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

func (s *Store) UpsertSystemOwnedApp(ctx context.Context, tenantID string, req apps.AppUpsert) (apps.App, error) {
	if req.PackageName == "" || req.Name == "" {
		return apps.App{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, system_owned
		 FROM apps
		 WHERE tenant_id = $1 AND package_name = $2`,
		tenantID, req.PackageName,
	)
	var systemOwned bool
	err := row.Scan(new(string), &systemOwned)
	switch {
	case err == nil && !systemOwned:
		return apps.App{}, httpx.ErrForbidden
	case err == nil:
		row = s.pool.QueryRow(ctx,
			`UPDATE apps
			 SET name = $3,
			     status = $4,
			     updated_at = $5
			 WHERE tenant_id = $1 AND package_name = $2 AND system_owned = TRUE
			 RETURNING id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at`,
			tenantID, req.PackageName, req.Name, apps.StatusActive, s.now(),
		)
	case errors.Is(err, pgx.ErrNoRows):
		row = s.pool.QueryRow(ctx,
			`INSERT INTO apps (id, tenant_id, package_name, name, system_owned, status, updated_at)
				 VALUES ($1, $2, $3, $4, TRUE, $5, $6)
				 RETURNING id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at`,
			uuid.NewString(), tenantID, req.PackageName, req.Name, apps.StatusActive, s.now(),
		)
	default:
		return apps.App{}, err
	}
	rec, err := scanApp(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apps.App{}, httpx.ErrConflict
		}
		return apps.App{}, err
	}
	return rec, nil
}

func (s *Store) CreateApp(ctx context.Context, tenantID string, req apps.AppUpsert) (apps.App, error) {
	if req.PackageName == "" || req.Name == "" {
		return apps.App{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO apps (id, tenant_id, package_name, name, system_owned, updated_at)
		 VALUES ($1, $2, $3, $4, FALSE, $5)
		 RETURNING id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at`,
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

func (s *Store) ListApps(ctx context.Context, tenantID string, page pagination.Params) ([]apps.App, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at
		 FROM apps
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
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

func (s *Store) GetOverviewStats(ctx context.Context, tenantID string) (apps.OverviewStats, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = $2)::int
		 FROM apps
		 WHERE tenant_id = $1`,
		tenantID, apps.StatusActive,
	)
	var stats apps.OverviewStats
	if err := row.Scan(&stats.Total, &stats.Active); err != nil {
		return apps.OverviewStats{}, err
	}
	return stats, nil
}

func (s *Store) GetApp(ctx context.Context, tenantID, id string) (apps.App, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at
		 FROM apps
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
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

func (s *Store) GetAppByPackageName(ctx context.Context, tenantID, packageName string) (apps.App, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at
		 FROM apps
		 WHERE tenant_id = $1 AND package_name = $2`,
		tenantID, packageName,
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

func (s *Store) UpdateApp(ctx context.Context, tenantID, id string, req apps.AppUpsert) (apps.App, error) {
	if req.PackageName == "" || req.Name == "" {
		return apps.App{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE apps
		 SET package_name = $3, name = $4, updated_at = $5
		 WHERE tenant_id = $1 AND id = $2 AND system_owned = FALSE
		 RETURNING id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at`,
		tenantID, id, req.PackageName, req.Name, s.now(),
	)
	rec, err := scanApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if locked, lockErr := s.isSystemOwnedApp(ctx, tenantID, id); lockErr == nil && locked {
				return apps.App{}, httpx.ErrForbidden
			}
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
		 WHERE tenant_id = $1 AND id = $2 AND system_owned = FALSE
		 RETURNING id::text, tenant_id::text, package_name, name, system_owned, status, created_at, updated_at, deleted_at`,
		tenantID, id, apps.StatusRetired, s.now(),
	)
	rec, err := scanApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if locked, lockErr := s.isSystemOwnedApp(ctx, tenantID, id); lockErr == nil && locked {
				return apps.App{}, httpx.ErrForbidden
			}
			return apps.App{}, httpx.ErrNotFound
		}
		return apps.App{}, err
	}
	return rec, nil
}

func (s *Store) ListVersions(ctx context.Context, tenantID, appID string, page pagination.Params) ([]apps.Version, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	var appExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM apps
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, appID,
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
		 ORDER BY created_at DESC, id DESC
		 LIMIT $3 OFFSET $4`,
		tenantID, appID, page.Limit, page.Offset,
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

func (s *Store) GetVersionByCode(ctx context.Context, tenantID, appID string, versionCode int64) (apps.Version, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, app_id::text, status, version_name, version_code, artifact_id, checksum, published_at, created_at
		 FROM app_versions
		 WHERE tenant_id = $1 AND app_id = $2 AND version_code = $3
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		tenantID, appID, versionCode,
	)
	rec, err := scanVersion(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apps.Version{}, httpx.ErrNotFound
		}
		return apps.Version{}, err
	}
	return rec, nil
}

func (s *Store) GetLatestPublishedVersion(ctx context.Context, tenantID, appID string) (apps.Version, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, app_id::text, status, version_name, version_code, artifact_id, checksum, published_at, created_at
		 FROM app_versions
		 WHERE tenant_id = $1 AND app_id = $2 AND status = $3
		 ORDER BY COALESCE(published_at, created_at) DESC, created_at DESC, id DESC
		 LIMIT 1`,
		tenantID, appID, apps.VersionStatusPublished,
	)
	rec, err := scanVersion(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apps.Version{}, httpx.ErrNotFound
		}
		return apps.Version{}, err
	}
	return rec, nil
}

func (s *Store) GetVersion(ctx context.Context, tenantID, appID, versionID string) (apps.Version, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT v.id::text, v.tenant_id::text, v.app_id::text, v.status, v.version_name, v.version_code, v.artifact_id, v.checksum, v.published_at, v.created_at,
		        a.id::text, a.tenant_id::text, a.storage_key, a.checksum, a.size_bytes, a.mime_type, a.status, a.updated_at, a.deleted_at
		 FROM app_versions v
		 LEFT JOIN artifacts a ON a.tenant_id = v.tenant_id AND a.id::text = v.artifact_id
		 WHERE v.tenant_id = $1 AND v.app_id = $2 AND v.id = $3`,
		tenantID, appID, versionID,
	)
	var rec apps.Version
	var artifactID pgtype.Text
	var publishedAt pgtype.Timestamptz
	var artifact fieldsArtifact
	if err := row.Scan(
		&rec.ID, &rec.TenantID, &rec.AppID, &rec.Status, &rec.VersionName, &rec.VersionCode, &artifactID, &rec.Checksum, &publishedAt, &rec.CreatedAt,
		&artifact.ID, &artifact.TenantID, &artifact.StorageKey, &artifact.Checksum, &artifact.SizeBytes, &artifact.MimeType, &artifact.Status, &artifact.UpdatedAt, &artifact.DeletedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apps.Version{}, httpx.ErrNotFound
		}
		return apps.Version{}, err
	}
	if artifactID.Valid {
		value := artifactID.String
		rec.ArtifactID = &value
	}
	if publishedAt.Valid {
		rec.PublishedAt = &publishedAt.Time
	}
	if artifact.ID.Valid {
		rec.Artifact = &files.Artifact{
			RecordBase: files.RecordBase{
				ID:        artifact.ID.String,
				TenantID:  artifact.TenantID.String,
				Status:    artifact.Status.String,
				UpdatedAt: artifact.UpdatedAt.Time,
			},
			StorageKey: artifact.StorageKey.String,
			Checksum:   artifact.Checksum.String,
			SizeBytes:  artifact.SizeBytes.Int64,
			MimeType:   artifact.MimeType.String,
		}
		if artifact.DeletedAt.Valid {
			deletedAt := artifact.DeletedAt.Time
			rec.Artifact.DeletedAt = &deletedAt
		}
	}
	return rec, nil
}

type fieldsArtifact struct {
	ID         pgtype.Text
	TenantID   pgtype.Text
	StorageKey pgtype.Text
	Checksum   pgtype.Text
	SizeBytes  pgtype.Int8
	MimeType   pgtype.Text
	Status     pgtype.Text
	UpdatedAt  pgtype.Timestamptz
	DeletedAt  pgtype.Timestamptz
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
		var storedChecksum string
		if err := tx.QueryRow(ctx,
			`SELECT checksum
			 FROM artifacts
			 WHERE tenant_id = $1 AND id = $2`,
			tenantID, *req.ArtifactID,
		).Scan(&storedChecksum); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apps.Version{}, httpx.ErrNotFound
			}
			return apps.Version{}, err
		}
		if storedChecksum != req.Checksum {
			return apps.Version{}, httpx.ErrInvalidInput
		}
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
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.PackageName, &rec.Name, &rec.SystemOwned, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return apps.App{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func (s *Store) isSystemOwnedApp(ctx context.Context, tenantID, id string) (bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT system_owned
		 FROM apps
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	var locked bool
	if err := row.Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, httpx.ErrNotFound
		}
		return false, err
	}
	return locked, nil
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
