-- +goose Up
CREATE TABLE artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    storage_key text NOT NULL,
    checksum text NOT NULL,
    size_bytes bigint NOT NULL,
    mime_type text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, storage_key)
);

CREATE TABLE files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    name text NOT NULL,
    artifact_id uuid NOT NULL,
    checksum text NOT NULL,
    mime_type text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
);

ALTER TABLE files
    ADD CONSTRAINT fk_files_artifact
    FOREIGN KEY (tenant_id, artifact_id) REFERENCES artifacts(tenant_id, id);

CREATE INDEX idx_artifacts_tenant_checksum ON artifacts (tenant_id, checksum);
CREATE INDEX idx_artifacts_tenant_status ON artifacts (tenant_id, status);
CREATE INDEX idx_files_tenant_status ON files (tenant_id, status);

-- +goose Down
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS artifacts;
