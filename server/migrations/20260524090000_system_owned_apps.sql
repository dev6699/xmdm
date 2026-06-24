-- +goose Up
ALTER TABLE apps
    ADD COLUMN IF NOT EXISTS system_owned boolean NOT NULL DEFAULT false;

UPDATE apps
SET name = 'XMDM Agent',
    status = 'active',
    system_owned = true,
    updated_at = now()
WHERE tenant_id = '11111111-1111-1111-1111-111111111111'
  AND package_name = 'com.xmdm.launcher';

-- +goose Down
UPDATE apps
SET system_owned = false,
    updated_at = now()
WHERE tenant_id = '11111111-1111-1111-1111-111111111111'
  AND package_name = 'com.xmdm.launcher';

ALTER TABLE apps
    DROP COLUMN IF EXISTS system_owned;
