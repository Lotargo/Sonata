-- +goose Up

ALTER TABLE sonata.role_runs
    ADD CONSTRAINT role_runs_canonical_role_unique
    UNIQUE (owner_id, cognitive_run_id, phase, perspective);

-- +goose Down

ALTER TABLE sonata.role_runs
    DROP CONSTRAINT role_runs_canonical_role_unique;
