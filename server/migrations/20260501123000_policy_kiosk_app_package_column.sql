-- +goose Up
ALTER TABLE policies
ADD COLUMN IF NOT EXISTS kiosk_app_package text;

UPDATE policies
SET kiosk_app_package = ''
WHERE kiosk_app_package IS NULL;

ALTER TABLE policies
ALTER COLUMN kiosk_app_package SET DEFAULT '';

-- +goose Down
ALTER TABLE policies
ALTER COLUMN kiosk_app_package DROP DEFAULT;

ALTER TABLE policies
DROP COLUMN IF EXISTS kiosk_app_package;
