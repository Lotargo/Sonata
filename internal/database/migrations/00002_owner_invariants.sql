-- +goose Up

ALTER TABLE sonata.manifest_versions
    ADD CONSTRAINT manifest_versions_owner_identity_unique
    UNIQUE (owner_id, manifest_id, version);

ALTER TABLE sonata.user_manifests
    DROP CONSTRAINT user_manifests_manifest_id_version_fkey,
    ADD CONSTRAINT user_manifests_owner_manifest_version_fkey
        FOREIGN KEY (owner_id, manifest_id, version)
        REFERENCES sonata.manifest_versions(owner_id, manifest_id, version)
        ON DELETE RESTRICT;

ALTER TABLE sonata.role_runs
    ADD CONSTRAINT role_runs_owner_run_role_unique
    UNIQUE (owner_id, cognitive_run_id, id);

ALTER TABLE sonata.tool_calls
    DROP CONSTRAINT tool_calls_owner_id_role_run_id_fkey,
    ADD CONSTRAINT tool_calls_owner_run_role_fkey
        FOREIGN KEY (owner_id, cognitive_run_id, role_run_id)
        REFERENCES sonata.role_runs(owner_id, cognitive_run_id, id)
        ON DELETE CASCADE;

ALTER TABLE sonata.provider_usage
    DROP CONSTRAINT provider_usage_owner_id_role_run_id_fkey,
    ADD CONSTRAINT provider_usage_owner_run_role_fkey
        FOREIGN KEY (owner_id, cognitive_run_id, role_run_id)
        REFERENCES sonata.role_runs(owner_id, cognitive_run_id, id)
        ON DELETE CASCADE;

ALTER TABLE sonata.memory_items
    DROP CONSTRAINT memory_items_owner_id_conversation_id_fkey,
    ADD CONSTRAINT memory_items_owner_conversation_fkey
        FOREIGN KEY (owner_id, conversation_id)
        REFERENCES sonata.conversations(owner_id, id)
        ON DELETE CASCADE;

-- +goose Down

ALTER TABLE sonata.memory_items
    DROP CONSTRAINT memory_items_owner_conversation_fkey,
    ADD CONSTRAINT memory_items_owner_id_conversation_id_fkey
        FOREIGN KEY (owner_id, conversation_id)
        REFERENCES sonata.conversations(owner_id, id)
        ON DELETE SET NULL;

ALTER TABLE sonata.provider_usage
    DROP CONSTRAINT provider_usage_owner_run_role_fkey,
    ADD CONSTRAINT provider_usage_owner_id_role_run_id_fkey
        FOREIGN KEY (owner_id, role_run_id)
        REFERENCES sonata.role_runs(owner_id, id)
        ON DELETE CASCADE;

ALTER TABLE sonata.tool_calls
    DROP CONSTRAINT tool_calls_owner_run_role_fkey,
    ADD CONSTRAINT tool_calls_owner_id_role_run_id_fkey
        FOREIGN KEY (owner_id, role_run_id)
        REFERENCES sonata.role_runs(owner_id, id)
        ON DELETE CASCADE;

ALTER TABLE sonata.role_runs
    DROP CONSTRAINT role_runs_owner_run_role_unique;

ALTER TABLE sonata.user_manifests
    DROP CONSTRAINT user_manifests_owner_manifest_version_fkey,
    ADD CONSTRAINT user_manifests_manifest_id_version_fkey
        FOREIGN KEY (manifest_id, version)
        REFERENCES sonata.manifest_versions(manifest_id, version)
        ON DELETE RESTRICT;

ALTER TABLE sonata.manifest_versions
    DROP CONSTRAINT manifest_versions_owner_identity_unique;
