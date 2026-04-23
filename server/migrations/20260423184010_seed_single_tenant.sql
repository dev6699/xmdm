-- +goose Up
INSERT INTO tenants (id, name, status)
VALUES ('11111111-1111-1111-1111-111111111111', 'Default tenant', 'active')
ON CONFLICT (id) DO UPDATE
SET name = EXCLUDED.name,
    status = EXCLUDED.status,
    updated_at = now();

-- +goose Down
DELETE FROM tenants
WHERE id = '11111111-1111-1111-1111-111111111111';
