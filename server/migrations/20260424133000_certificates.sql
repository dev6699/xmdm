-- +goose Up
CREATE TABLE certificates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    name text NOT NULL,
    artifact_id uuid NOT NULL,
    checksum text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
);

ALTER TABLE certificates
    ADD CONSTRAINT fk_certificates_artifact
    FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id);

CREATE INDEX idx_certificates_tenant_status ON certificates (tenant_id, status);

-- +goose Down
DROP TABLE IF EXISTS certificates;
