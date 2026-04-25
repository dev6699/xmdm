-- +goose Up
ALTER TABLE commands
    ADD COLUMN acked_at timestamptz,
    ADD COLUMN result_json jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE commands
    DROP COLUMN IF EXISTS result_json,
    DROP COLUMN IF EXISTS acked_at;
