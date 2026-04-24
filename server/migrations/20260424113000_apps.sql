-- +goose Up
CREATE TABLE apps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    package_name text NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, package_name)
);

CREATE TABLE app_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    app_id uuid NOT NULL,
    version_name text NOT NULL,
    version_code bigint NOT NULL,
    artifact_id text,
    checksum text NOT NULL,
    status text NOT NULL DEFAULT 'uploaded',
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, app_id, version_name, version_code)
);

ALTER TABLE app_versions
    ADD CONSTRAINT fk_app_versions_app
    FOREIGN KEY (tenant_id, app_id) REFERENCES apps(tenant_id, id);

CREATE INDEX idx_apps_tenant_status ON apps (tenant_id, status);
CREATE INDEX idx_app_versions_tenant_app ON app_versions (tenant_id, app_id);
CREATE INDEX idx_app_versions_tenant_status ON app_versions (tenant_id, status);

-- +goose Down
DROP TABLE IF EXISTS app_versions;
DROP TABLE IF EXISTS apps;
