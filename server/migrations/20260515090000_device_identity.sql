-- +goose Up
ALTER TABLE devices
    DROP CONSTRAINT IF EXISTS devices_tenant_id_device_id_key;

ALTER TABLE devices
    DROP COLUMN IF EXISTS device_id;

-- +goose Down
ALTER TABLE devices
    ADD COLUMN device_id text NOT NULL DEFAULT '';

UPDATE devices
SET device_id = id::text;

ALTER TABLE devices
    ALTER COLUMN device_id DROP DEFAULT;

ALTER TABLE devices
    ADD CONSTRAINT devices_tenant_id_device_id_key UNIQUE (tenant_id, device_id);
