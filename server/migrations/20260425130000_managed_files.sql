-- +goose Up
CREATE TABLE managed_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    file_id uuid NOT NULL,
    path text NOT NULL,
    replace_variables boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, path)
);

ALTER TABLE managed_files
    ADD CONSTRAINT fk_managed_files_source_file
    FOREIGN KEY (tenant_id, file_id) REFERENCES files(tenant_id, id);

CREATE INDEX idx_managed_files_tenant_status ON managed_files (tenant_id, status);

-- +goose Down
DROP TABLE IF EXISTS managed_files;
