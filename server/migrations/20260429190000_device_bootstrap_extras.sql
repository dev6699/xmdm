-- +goose Up
ALTER TABLE devices
    ADD COLUMN bootstrap_extras jsonb;

-- +goose Down
ALTER TABLE devices
    DROP COLUMN IF EXISTS bootstrap_extras;
