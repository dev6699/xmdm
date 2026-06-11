package policypg

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"xmdm/server/internal/apps"
	"xmdm/server/internal/certificates"
	"xmdm/server/internal/httpx"
	managedfiles "xmdm/server/internal/managedfiles"
	"xmdm/server/internal/pagination"
	policy "xmdm/server/internal/policy"

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

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, now: time.Now} }

func (s *Store) SetNow(now func() time.Time) { s.now = now }

func (s *Store) CreatePolicy(ctx context.Context, tenantID string, req policy.PolicyUpsert) (policy.Policy, error) {
	if req.Name == "" {
		return policy.Policy{}, httpx.ErrInvalidInput
	}
	restrictions := req.Restrictions
	if len(restrictions) == 0 || string(restrictions) == "null" {
		restrictions = []byte(`{}`)
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO policies (id, tenant_id, name, version, kiosk_mode, kiosk_app_package, restrictions_json, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
		 RETURNING id::text, tenant_id::text, name, version, kiosk_mode, kiosk_app_package, restrictions_json, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, req.Name, 1, req.KioskMode, req.KioskAppPackage, restrictions, s.now(),
	)
	return scanPolicy(row)
}

func (s *Store) ListPolicies(ctx context.Context, tenantID string, page pagination.Params) ([]policy.Policy, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, version, kiosk_mode, kiosk_app_package, restrictions_json, status, created_at, updated_at, deleted_at
		 FROM policies
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC, id DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.Policy, 0)
	for rows.Next() {
		rec, err := scanPolicy(rows)
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

func (s *Store) ListActivePolicies(ctx context.Context, tenantID string) ([]policy.Policy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, name, version, kiosk_mode, kiosk_app_package, restrictions_json, status, created_at, updated_at, deleted_at
		 FROM policies
		 WHERE tenant_id = $1 AND status = $2
		 ORDER BY created_at DESC, id DESC`,
		tenantID, policy.StatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.Policy, 0)
	for rows.Next() {
		rec, err := scanPolicy(rows)
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

func (s *Store) GetOverviewStats(ctx context.Context, tenantID string) (policy.OverviewStats, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = $2)::int,
			COUNT(*) FILTER (WHERE status = $3)::int
		 FROM policies
		 WHERE tenant_id = $1`,
		tenantID, policy.StatusActive, policy.StatusRetired,
	)
	var stats policy.OverviewStats
	if err := row.Scan(&stats.Total, &stats.Active, &stats.Retired); err != nil {
		return policy.OverviewStats{}, err
	}
	return stats, nil
}

func (s *Store) GetPolicy(ctx context.Context, tenantID, id string) (policy.Policy, error) {
	if tenantID == "" || id == "" {
		return policy.Policy{}, httpx.ErrInvalidInput
	}
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, name, version, kiosk_mode, kiosk_app_package, restrictions_json, status, created_at, updated_at, deleted_at
		 FROM policies
		 WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	)
	rec, err := scanPolicy(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.Policy{}, httpx.ErrNotFound
		}
		return policy.Policy{}, err
	}
	return rec, nil
}

func (s *Store) UpdatePolicy(ctx context.Context, tenantID, id string, req policy.PolicyUpsert) (policy.Policy, error) {
	if req.Name == "" {
		return policy.Policy{}, httpx.ErrInvalidInput
	}
	restrictions := req.Restrictions
	if len(restrictions) == 0 || string(restrictions) == "null" {
		restrictions = []byte(`{}`)
	}
	if req.KioskMode {
		var existingRestrictions json.RawMessage
		var existingKioskMode bool
		if err := s.pool.QueryRow(ctx,
			`SELECT kiosk_mode, restrictions_json
			 FROM policies
			 WHERE tenant_id = $1 AND id = $2`,
			tenantID, id,
		).Scan(&existingKioskMode, &existingRestrictions); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return policy.Policy{}, httpx.ErrNotFound
			}
			return policy.Policy{}, err
		}
		if existingKioskMode && !kioskExitPasscodeConfigured(restrictions) {
			restrictions = mergeKioskExitPasscode(existingRestrictions, restrictions)
		}
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE policies
		 SET name = $3, version = version + 1, kiosk_mode = $4, kiosk_app_package = $5, restrictions_json = $6::jsonb, updated_at = $7
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, version, kiosk_mode, kiosk_app_package, restrictions_json, status, created_at, updated_at, deleted_at`,
		tenantID, id, req.Name, req.KioskMode, req.KioskAppPackage, restrictions, s.now(),
	)
	rec, err := scanPolicy(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.Policy{}, httpx.ErrNotFound
		}
		return policy.Policy{}, err
	}
	return rec, nil
}

func mergeKioskExitPasscode(existing, next json.RawMessage) json.RawMessage {
	var current policyRestrictionsPayload
	if len(existing) > 0 && string(existing) != "null" {
		_ = json.Unmarshal(existing, &current)
	}
	var updated policyRestrictionsPayload
	if len(next) > 0 && string(next) != "null" {
		_ = json.Unmarshal(next, &updated)
	}
	if strings.TrimSpace(updated.KioskExitPasscode) == "" {
		updated.KioskExitPasscode = strings.TrimSpace(current.KioskExitPasscode)
		if updated.KioskExitPasscode == "" {
			updated.KioskExitPasscodeHash = strings.TrimSpace(current.KioskExitPasscodeHash)
		}
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		return next
	}
	return raw
}

func (s *Store) RetirePolicy(ctx context.Context, tenantID, id string) (policy.Policy, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE policies
		 SET status = 'retired', deleted_at = $3, updated_at = $3
		 WHERE tenant_id = $1 AND id = $2
		 RETURNING id::text, tenant_id::text, name, version, kiosk_mode, kiosk_app_package, restrictions_json, status, created_at, updated_at, deleted_at`,
		tenantID, id, s.now(),
	)
	rec, err := scanPolicy(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.Policy{}, httpx.ErrNotFound
		}
		return policy.Policy{}, err
	}
	return rec, nil
}

func (s *Store) ListPolicyApps(ctx context.Context, tenantID, policyID string, page pagination.Params) ([]policy.PolicyApp, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, app_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_apps
		 WHERE tenant_id = $1 AND policy_id = $2
		 ORDER BY created_at, id
		 LIMIT $3 OFFSET $4`,
		tenantID, policyID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]policy.PolicyApp, 0)
	for rows.Next() {
		rec, err := scanPolicyApp(rows)
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

func (s *Store) ListActivePolicyApps(ctx context.Context, tenantID, policyID string) ([]policy.PolicyApp, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, app_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_apps
		 WHERE tenant_id = $1 AND policy_id = $2 AND status = $3
		 ORDER BY created_at, id`,
		tenantID, policyID, policy.StatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.PolicyApp, 0)
	for rows.Next() {
		rec, err := scanPolicyApp(rows)
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

func (s *Store) GetPolicyApp(ctx context.Context, tenantID, policyID, appID string) (policy.PolicyApp, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, app_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_apps
		 WHERE tenant_id = $1 AND policy_id = $2 AND app_id = $3`,
		tenantID, policyID, appID,
	)
	rec, err := scanPolicyApp(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyApp{}, httpx.ErrNotFound
		}
		return policy.PolicyApp{}, err
	}
	return rec, nil
}

func (s *Store) AddPolicyApp(ctx context.Context, tenantID, policyID, appID string) (policy.PolicyApp, error) {
	if tenantID == "" || policyID == "" || appID == "" {
		return policy.PolicyApp{}, httpx.ErrInvalidInput
	}
	var policyExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM policies
		 WHERE tenant_id = $1 AND id = $2 AND status <> 'retired'`,
		tenantID, policyID,
	).Scan(&policyExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyApp{}, httpx.ErrNotFound
		}
		return policy.PolicyApp{}, err
	}
	var appExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM apps
		 WHERE tenant_id = $1 AND id = $2 AND status <> $3`,
		tenantID, appID, apps.StatusRetired,
	).Scan(&appExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyApp{}, httpx.ErrNotFound
		}
		return policy.PolicyApp{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO policy_apps (id, tenant_id, policy_id, app_id, status, created_at, updated_at, deleted_at)
		 VALUES ($1, $2, $3, $4, 'active', $5, $5, NULL)
		 ON CONFLICT (tenant_id, policy_id, app_id)
		 DO UPDATE SET status = 'active', updated_at = EXCLUDED.updated_at, deleted_at = NULL
		 RETURNING id::text, tenant_id::text, policy_id::text, app_id::text, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, policyID, appID, s.now(),
	)
	rec, err := scanPolicyApp(row)
	if err != nil {
		if isUniqueViolation(err) {
			return policy.PolicyApp{}, httpx.ErrConflict
		}
		return policy.PolicyApp{}, err
	}
	return rec, nil
}

func (s *Store) RemovePolicyApp(ctx context.Context, tenantID, policyID, appID string) error {
	res, err := s.pool.Exec(ctx,
		`UPDATE policy_apps
		 SET status = 'disabled', updated_at = $4, deleted_at = $4
		 WHERE tenant_id = $1 AND policy_id = $2 AND app_id = $3`,
		tenantID, policyID, appID, s.now(),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (s *Store) ListPolicyCertificates(ctx context.Context, tenantID, policyID string, page pagination.Params) ([]policy.PolicyCertificate, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, certificate_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_certificates
		 WHERE tenant_id = $1 AND policy_id = $2
		 ORDER BY created_at, id
		 LIMIT $3 OFFSET $4`,
		tenantID, policyID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]policy.PolicyCertificate, 0)
	for rows.Next() {
		rec, err := scanPolicyCertificate(rows)
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

func (s *Store) ListActivePolicyCertificates(ctx context.Context, tenantID, policyID string) ([]policy.PolicyCertificate, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, certificate_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_certificates
		 WHERE tenant_id = $1 AND policy_id = $2 AND status = $3
		 ORDER BY created_at, id`,
		tenantID, policyID, policy.StatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.PolicyCertificate, 0)
	for rows.Next() {
		rec, err := scanPolicyCertificate(rows)
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

func (s *Store) GetPolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) (policy.PolicyCertificate, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, certificate_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_certificates
		 WHERE tenant_id = $1 AND policy_id = $2 AND certificate_id = $3`,
		tenantID, policyID, certificateID,
	)
	rec, err := scanPolicyCertificate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyCertificate{}, httpx.ErrNotFound
		}
		return policy.PolicyCertificate{}, err
	}
	return rec, nil
}

func (s *Store) AddPolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) (policy.PolicyCertificate, error) {
	if tenantID == "" || policyID == "" || certificateID == "" {
		return policy.PolicyCertificate{}, httpx.ErrInvalidInput
	}
	var policyExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM policies
		 WHERE tenant_id = $1 AND id = $2 AND status <> 'retired'`,
		tenantID, policyID,
	).Scan(&policyExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyCertificate{}, httpx.ErrNotFound
		}
		return policy.PolicyCertificate{}, err
	}
	var certificateExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM certificates
		 WHERE tenant_id = $1 AND id = $2 AND status <> $3`,
		tenantID, certificateID, certificates.StatusRetired,
	).Scan(&certificateExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyCertificate{}, httpx.ErrNotFound
		}
		return policy.PolicyCertificate{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO policy_certificates (id, tenant_id, policy_id, certificate_id, status, created_at, updated_at, deleted_at)
		 VALUES ($1, $2, $3, $4, 'active', $5, $5, NULL)
		 ON CONFLICT (tenant_id, policy_id, certificate_id)
		 DO UPDATE SET status = 'active', updated_at = EXCLUDED.updated_at, deleted_at = NULL
		 RETURNING id::text, tenant_id::text, policy_id::text, certificate_id::text, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, policyID, certificateID, s.now(),
	)
	rec, err := scanPolicyCertificate(row)
	if err != nil {
		if isUniqueViolation(err) {
			return policy.PolicyCertificate{}, httpx.ErrConflict
		}
		return policy.PolicyCertificate{}, err
	}
	return rec, nil
}

func (s *Store) RemovePolicyCertificate(ctx context.Context, tenantID, policyID, certificateID string) error {
	res, err := s.pool.Exec(ctx,
		`UPDATE policy_certificates
		 SET status = 'disabled', updated_at = $4, deleted_at = $4
		 WHERE tenant_id = $1 AND policy_id = $2 AND certificate_id = $3`,
		tenantID, policyID, certificateID, s.now(),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func (s *Store) ListPolicyManagedFiles(ctx context.Context, tenantID, policyID string, page pagination.Params) ([]policy.PolicyManagedFile, error) {
	page = pagination.Normalize(page, pagination.DefaultLimit, 100)
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, managed_file_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_managed_files
		 WHERE tenant_id = $1 AND policy_id = $2
		 ORDER BY created_at, id
		 LIMIT $3 OFFSET $4`,
		tenantID, policyID, page.Limit, page.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]policy.PolicyManagedFile, 0)
	for rows.Next() {
		rec, err := scanPolicyManagedFile(rows)
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

func (s *Store) ListActivePolicyManagedFiles(ctx context.Context, tenantID, policyID string) ([]policy.PolicyManagedFile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, managed_file_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_managed_files
		 WHERE tenant_id = $1 AND policy_id = $2 AND status = $3
		 ORDER BY created_at, id`,
		tenantID, policyID, policy.StatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.PolicyManagedFile, 0)
	for rows.Next() {
		rec, err := scanPolicyManagedFile(rows)
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

func (s *Store) GetPolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) (policy.PolicyManagedFile, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text, policy_id::text, managed_file_id::text, status, created_at, updated_at, deleted_at
		 FROM policy_managed_files
		 WHERE tenant_id = $1 AND policy_id = $2 AND managed_file_id = $3`,
		tenantID, policyID, managedFileID,
	)
	rec, err := scanPolicyManagedFile(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyManagedFile{}, httpx.ErrNotFound
		}
		return policy.PolicyManagedFile{}, err
	}
	return rec, nil
}

func (s *Store) AddPolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) (policy.PolicyManagedFile, error) {
	if tenantID == "" || policyID == "" || managedFileID == "" {
		return policy.PolicyManagedFile{}, httpx.ErrInvalidInput
	}
	var policyExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM policies
		 WHERE tenant_id = $1 AND id = $2 AND status <> 'retired'`,
		tenantID, policyID,
	).Scan(&policyExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyManagedFile{}, httpx.ErrNotFound
		}
		return policy.PolicyManagedFile{}, err
	}
	var bindingExists string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text
		 FROM managed_files
		 WHERE tenant_id = $1 AND id = $2 AND status <> $3`,
		tenantID, managedFileID, managedfiles.StatusRetired,
	).Scan(&bindingExists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return policy.PolicyManagedFile{}, httpx.ErrNotFound
		}
		return policy.PolicyManagedFile{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO policy_managed_files (id, tenant_id, policy_id, managed_file_id, status, created_at, updated_at, deleted_at)
		 VALUES ($1, $2, $3, $4, 'active', $5, $5, NULL)
		 ON CONFLICT (tenant_id, policy_id, managed_file_id)
		 DO UPDATE SET status = 'active', updated_at = EXCLUDED.updated_at, deleted_at = NULL
		 RETURNING id::text, tenant_id::text, policy_id::text, managed_file_id::text, status, created_at, updated_at, deleted_at`,
		uuid.NewString(), tenantID, policyID, managedFileID, s.now(),
	)
	rec, err := scanPolicyManagedFile(row)
	if err != nil {
		if isUniqueViolation(err) {
			return policy.PolicyManagedFile{}, httpx.ErrConflict
		}
		return policy.PolicyManagedFile{}, err
	}
	return rec, nil
}

func (s *Store) RemovePolicyManagedFile(ctx context.Context, tenantID, policyID, managedFileID string) error {
	res, err := s.pool.Exec(ctx,
		`UPDATE policy_managed_files
		 SET status = 'disabled', updated_at = $4, deleted_at = $4
		 WHERE tenant_id = $1 AND policy_id = $2 AND managed_file_id = $3`,
		tenantID, policyID, managedFileID, s.now(),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

func scanPolicy(scanner rowScanner) (policy.Policy, error) {
	var rec policy.Policy
	var createdAt time.Time
	var deletedAt pgtype.Timestamptz
	var kioskAppPackage pgtype.Text
	var restrictions json.RawMessage
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.Version, &rec.KioskMode, &kioskAppPackage, &restrictions, &rec.Status, &createdAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return policy.Policy{}, err
	}
	rec.CreatedAt = createdAt
	if kioskAppPackage.Valid {
		rec.KioskAppPackage = kioskAppPackage.String
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	if len(restrictions) > 0 {
		rec.Restrictions = append(json.RawMessage(nil), restrictions...)
	} else {
		rec.Restrictions = json.RawMessage(`{}`)
	}
	return rec, nil
}

type policyRestrictionsPayload struct {
	AllowPackages                []string `json:"allowPackages,omitempty"`
	BlockPackages                []string `json:"blockPackages,omitempty"`
	SuspendPackages              []string `json:"suspendPackages,omitempty"`
	KioskKeepScreenOn            bool     `json:"kioskKeepScreenOn,omitempty"`
	KioskStayAwakeWhilePluggedIn bool     `json:"kioskStayAwakeWhilePluggedIn,omitempty"`
	KioskUnlockOnBoot            bool     `json:"kioskUnlockOnBoot,omitempty"`
	KioskExitPasscode            string   `json:"kioskExitPasscode,omitempty"`
	KioskExitPasscodeHash        string   `json:"kioskExitPasscodeHash,omitempty"`
}

func kioskExitPasscodeConfigured(restrictions json.RawMessage) bool {
	if len(restrictions) == 0 || string(restrictions) == "null" {
		return false
	}
	var parsed policyRestrictionsPayload
	if err := json.Unmarshal(restrictions, &parsed); err != nil {
		return false
	}
	return strings.TrimSpace(parsed.KioskExitPasscode) != "" || strings.TrimSpace(parsed.KioskExitPasscodeHash) != ""
}

func scanPolicyApp(scanner rowScanner) (policy.PolicyApp, error) {
	var rec policy.PolicyApp
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.PolicyID, &rec.AppID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return policy.PolicyApp{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanPolicyCertificate(scanner rowScanner) (policy.PolicyCertificate, error) {
	var rec policy.PolicyCertificate
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.PolicyID, &rec.CertificateID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return policy.PolicyCertificate{}, err
	}
	if deletedAt.Valid {
		rec.DeletedAt = &deletedAt.Time
	}
	return rec, nil
}

func scanPolicyManagedFile(scanner rowScanner) (policy.PolicyManagedFile, error) {
	var rec policy.PolicyManagedFile
	var deletedAt pgtype.Timestamptz
	if err := scanner.Scan(&rec.ID, &rec.TenantID, &rec.PolicyID, &rec.ManagedFileID, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt, &deletedAt); err != nil {
		return policy.PolicyManagedFile{}, err
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
