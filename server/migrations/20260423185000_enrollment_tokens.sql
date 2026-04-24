-- +goose Up
CREATE TABLE enrollment_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    token_hash text NOT NULL,
    status text NOT NULL DEFAULT 'issued',
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, token_hash)
);

CREATE INDEX idx_enrollment_tokens_tenant_status ON enrollment_tokens (tenant_id, status);
CREATE INDEX idx_enrollment_tokens_tenant_expires_at ON enrollment_tokens (tenant_id, expires_at);

-- +goose Down
DROP TABLE IF EXISTS enrollment_tokens;
