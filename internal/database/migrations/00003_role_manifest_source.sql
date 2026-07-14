-- +goose Up

ALTER TABLE sonata.role_runs
    ADD COLUMN manifest_source text NOT NULL DEFAULT 'system_default'
    CHECK (manifest_source IN ('system_default', 'user_global', 'user_chat'));

-- +goose Down

ALTER TABLE sonata.role_runs
    DROP COLUMN manifest_source;
