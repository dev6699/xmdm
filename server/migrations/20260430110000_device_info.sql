-- +goose Up
CREATE TABLE device_info (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    device_id uuid NOT NULL,
    observed_at timestamptz NOT NULL,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, device_id) REFERENCES devices(tenant_id, id)
);

CREATE INDEX idx_device_info_tenant_device_observed ON device_info (tenant_id, device_id, observed_at DESC);
CREATE INDEX idx_device_info_tenant_observed ON device_info (tenant_id, observed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS device_info;
