-- +goose Up
CREATE TABLE commands (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    device_id uuid NOT NULL,
    type text NOT NULL,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'queued',
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id)
);

CREATE INDEX idx_commands_tenant_device_status_created ON commands (tenant_id, device_id, status, created_at);

-- +goose Down
DROP TABLE IF EXISTS commands;
