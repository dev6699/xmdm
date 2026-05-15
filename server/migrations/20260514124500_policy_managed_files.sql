-- +goose Up
CREATE TABLE policy_managed_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    policy_id uuid NOT NULL,
    managed_file_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, policy_id, managed_file_id),
    CONSTRAINT fk_policy_managed_files_policy FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id),
    CONSTRAINT fk_policy_managed_files_managed_file FOREIGN KEY (tenant_id, managed_file_id) REFERENCES managed_files(tenant_id, id)
);

CREATE INDEX idx_policy_managed_files_tenant_policy ON policy_managed_files (tenant_id, policy_id);
CREATE INDEX idx_policy_managed_files_tenant_managed_file ON policy_managed_files (tenant_id, managed_file_id);

-- +goose Down
DROP TABLE IF EXISTS policy_managed_files;
