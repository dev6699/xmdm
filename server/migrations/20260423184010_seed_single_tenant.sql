-- +goose Up
INSERT INTO tenants (id, name, status)
VALUES ('11111111-1111-1111-1111-111111111111', 'Default tenant', 'active')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

INSERT INTO roles (id, tenant_id, name, permissions, status)
VALUES (
    '22222222-2222-2222-2222-222222222222',
    '11111111-1111-1111-1111-111111111111',
    'admins',
    '["admin.read","admin.write","devices.read","devices.write"]'::jsonb,
    'active'
)
ON CONFLICT (tenant_id, id) DO UPDATE
SET name = EXCLUDED.name,
    permissions = EXCLUDED.permissions,
    status = EXCLUDED.status,
    updated_at = now();

-- +goose Down
DELETE FROM roles
WHERE id = '22222222-2222-2222-2222-222222222222';

DELETE FROM tenants
WHERE id = '11111111-1111-1111-1111-111111111111';
