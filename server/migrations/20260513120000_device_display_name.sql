-- +goose Up
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';

UPDATE devices
SET display_name = device_id
WHERE display_name = '';

-- +goose Down
ALTER TABLE devices
    DROP COLUMN IF EXISTS display_name;
