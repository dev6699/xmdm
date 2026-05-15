-- +goose Up
CREATE TABLE policy_apps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    policy_id uuid NOT NULL,
    app_id uuid NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, policy_id, app_id),
    CONSTRAINT fk_policy_apps_policy FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id),
    CONSTRAINT fk_policy_apps_app FOREIGN KEY (tenant_id, app_id) REFERENCES apps(tenant_id, id)
);

CREATE INDEX idx_policy_apps_tenant_policy ON policy_apps (tenant_id, policy_id);
CREATE INDEX idx_policy_apps_tenant_app ON policy_apps (tenant_id, app_id);

-- +goose Down
DROP TABLE IF EXISTS policy_apps;
