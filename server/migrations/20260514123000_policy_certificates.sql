-- +goose Up
CREATE TABLE policy_certificates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    policy_id uuid NOT NULL,
    certificate_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, policy_id, certificate_id),
    CONSTRAINT fk_policy_certificates_policy FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id),
    CONSTRAINT fk_policy_certificates_certificate FOREIGN KEY (tenant_id, certificate_id) REFERENCES certificates(tenant_id, id)
);

CREATE INDEX idx_policy_certificates_tenant_policy ON policy_certificates (tenant_id, policy_id);
CREATE INDEX idx_policy_certificates_tenant_certificate ON policy_certificates (tenant_id, certificate_id);

-- +goose Down
DROP TABLE IF EXISTS policy_certificates;
