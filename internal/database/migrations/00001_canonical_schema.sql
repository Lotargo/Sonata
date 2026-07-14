-- +goose Up

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE SCHEMA IF NOT EXISTS sonata;

CREATE TABLE sonata.users (
    id text PRIMARY KEY CHECK (btrim(id) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sonata.conversations (
    owner_id text NOT NULL REFERENCES sonata.users(id) ON DELETE CASCADE,
    id text NOT NULL CHECK (btrim(id) <> ''),
    title text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, id)
);

CREATE TABLE sonata.messages (
    owner_id text NOT NULL,
    conversation_id text NOT NULL,
    id text NOT NULL CHECK (btrim(id) <> ''),
    role text NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    content jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, id),
    UNIQUE (owner_id, id, conversation_id),
    FOREIGN KEY (owner_id, conversation_id)
        REFERENCES sonata.conversations(owner_id, id)
        ON DELETE CASCADE
);

CREATE INDEX messages_conversation_created_idx
    ON sonata.messages(owner_id, conversation_id, created_at, id);

CREATE TABLE sonata.cognitive_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL,
    conversation_id text NOT NULL,
    request_message_id text NOT NULL,
    route text NOT NULL CHECK (route IN ('direct', 'full')),
    status text NOT NULL CHECK (status IN (
        'RUNNING', 'OK', 'DEGRADED', 'PROVIDER_EXHAUSTED',
        'FAILED_ROUTING', 'FAILED_CONTEXT', 'FAILED_TOOLING', 'FAILED_SYNTHESIS'
    )),
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (owner_id, id),
    FOREIGN KEY (owner_id, conversation_id)
        REFERENCES sonata.conversations(owner_id, id)
        ON DELETE CASCADE,
    FOREIGN KEY (owner_id, request_message_id, conversation_id)
        REFERENCES sonata.messages(owner_id, id, conversation_id)
        ON DELETE RESTRICT,
    CHECK (completed_at IS NULL OR completed_at >= started_at)
);

CREATE INDEX cognitive_runs_owner_started_idx
    ON sonata.cognitive_runs(owner_id, started_at DESC, id);

CREATE TABLE sonata.role_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL,
    cognitive_run_id uuid NOT NULL,
    phase text NOT NULL CHECK (phase IN (
        'router', 'raw', 'critical', 'summary', 'synthesis_tooling', 'synthesis_final'
    )),
    perspective text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('RUNNING', 'OK', 'DEGRADED', 'FAILED')),
    model_id text NOT NULL DEFAULT '',
    instruction_id text NOT NULL DEFAULT '',
    instruction_version integer NOT NULL DEFAULT 0 CHECK (instruction_version >= 0),
    instruction_hash text NOT NULL DEFAULT '',
    manifest_id text NOT NULL DEFAULT '',
    manifest_version integer NOT NULL DEFAULT 0 CHECK (manifest_version >= 0),
    manifest_hash text NOT NULL DEFAULT '',
    latency_ms bigint NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    usage jsonb NOT NULL DEFAULT '{}'::jsonb,
    error_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, id),
    FOREIGN KEY (owner_id, cognitive_run_id)
        REFERENCES sonata.cognitive_runs(owner_id, id)
        ON DELETE CASCADE
);

CREATE INDEX role_runs_cognitive_run_idx
    ON sonata.role_runs(owner_id, cognitive_run_id, created_at, id);

CREATE TABLE sonata.tool_calls (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL,
    cognitive_run_id uuid NOT NULL,
    role_run_id uuid NOT NULL,
    tool_name text NOT NULL CHECK (btrim(tool_name) <> ''),
    status text NOT NULL CHECK (status IN ('PLANNED', 'RUNNING', 'OK', 'FAILED', 'REJECTED')),
    request_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    result_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    FOREIGN KEY (owner_id, cognitive_run_id)
        REFERENCES sonata.cognitive_runs(owner_id, id)
        ON DELETE CASCADE,
    FOREIGN KEY (owner_id, role_run_id)
        REFERENCES sonata.role_runs(owner_id, id)
        ON DELETE CASCADE,
    CHECK (completed_at IS NULL OR completed_at >= created_at)
);

CREATE INDEX tool_calls_cognitive_run_idx
    ON sonata.tool_calls(owner_id, cognitive_run_id, created_at, id);

CREATE TABLE sonata.instruction_versions (
    instruction_id text NOT NULL CHECK (btrim(instruction_id) <> ''),
    version integer NOT NULL CHECK (version > 0),
    content_hash text NOT NULL CHECK (btrim(content_hash) <> ''),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (instruction_id, version),
    UNIQUE (instruction_id, content_hash)
);

CREATE TABLE sonata.manifest_versions (
    manifest_id text NOT NULL CHECK (btrim(manifest_id) <> ''),
    version integer NOT NULL CHECK (version > 0),
    owner_id text REFERENCES sonata.users(id) ON DELETE CASCADE,
    source text NOT NULL CHECK (source IN ('system_default', 'user_global', 'user_chat')),
    content_hash text NOT NULL CHECK (btrim(content_hash) <> ''),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (manifest_id, version),
    CHECK (
        (source = 'system_default' AND owner_id IS NULL)
        OR (source <> 'system_default' AND owner_id IS NOT NULL)
    )
);

CREATE TABLE sonata.user_manifests (
    owner_id text NOT NULL REFERENCES sonata.users(id) ON DELETE CASCADE,
    scope text NOT NULL CHECK (scope IN ('global', 'chat')),
    scope_id text NOT NULL DEFAULT '',
    manifest_id text NOT NULL CHECK (btrim(manifest_id) <> ''),
    version integer NOT NULL CHECK (version > 0),
    status text NOT NULL CHECK (status IN ('active', 'disabled', 'deleted', 'rejected')),
    content text NOT NULL,
    content_hash text NOT NULL CHECK (btrim(content_hash) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_id, scope, scope_id),
    UNIQUE (manifest_id, version),
    FOREIGN KEY (manifest_id, version)
        REFERENCES sonata.manifest_versions(manifest_id, version)
        ON DELETE RESTRICT,
    CHECK (
        (scope = 'global' AND scope_id = '')
        OR (scope = 'chat' AND btrim(scope_id) <> '')
    )
);

CREATE TABLE sonata.affective_states (
    identity_id text NOT NULL CHECK (btrim(identity_id) <> ''),
    owner_id text NOT NULL REFERENCES sonata.users(id) ON DELETE CASCADE,
    version bigint NOT NULL CHECK (version >= 0),
    profile_version text NOT NULL CHECK (btrim(profile_version) <> ''),
    state jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (identity_id, owner_id)
);

CREATE INDEX affective_states_owner_updated_idx
    ON sonata.affective_states(owner_id, updated_at DESC, identity_id);

CREATE TABLE sonata.affective_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    identity_id text NOT NULL,
    owner_id text NOT NULL,
    state_version bigint NOT NULL CHECK (state_version > 0),
    kind text NOT NULL CHECK (btrim(kind) <> ''),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (identity_id, owner_id)
        REFERENCES sonata.affective_states(identity_id, owner_id)
        ON DELETE CASCADE,
    UNIQUE (identity_id, owner_id, state_version, kind)
);

CREATE INDEX affective_events_owner_version_idx
    ON sonata.affective_events(owner_id, identity_id, state_version, created_at);

CREATE TABLE sonata.memory_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES sonata.users(id) ON DELETE CASCADE,
    conversation_id text,
    kind text NOT NULL CHECK (btrim(kind) <> ''),
    content text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    projection_status text NOT NULL DEFAULT 'pending'
        CHECK (projection_status IN ('pending', 'indexed', 'failed', 'deleted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, id),
    FOREIGN KEY (owner_id, conversation_id)
        REFERENCES sonata.conversations(owner_id, id)
        ON DELETE SET NULL
);

CREATE INDEX memory_items_owner_created_idx
    ON sonata.memory_items(owner_id, created_at DESC, id);

CREATE TABLE sonata.documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES sonata.users(id) ON DELETE CASCADE,
    filename text NOT NULL CHECK (btrim(filename) <> ''),
    media_type text NOT NULL DEFAULT 'application/octet-stream',
    object_key text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('pending', 'ready', 'failed', 'deleted')),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, id)
);

CREATE INDEX documents_owner_created_idx
    ON sonata.documents(owner_id, created_at DESC, id);

CREATE TABLE sonata.provider_usage (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES sonata.users(id) ON DELETE CASCADE,
    cognitive_run_id uuid NOT NULL,
    role_run_id uuid NOT NULL,
    provider text NOT NULL CHECK (btrim(provider) <> ''),
    model_id text NOT NULL CHECK (btrim(model_id) <> ''),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cached_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (owner_id, cognitive_run_id)
        REFERENCES sonata.cognitive_runs(owner_id, id)
        ON DELETE CASCADE,
    FOREIGN KEY (owner_id, role_run_id)
        REFERENCES sonata.role_runs(owner_id, id)
        ON DELETE CASCADE
);

CREATE INDEX provider_usage_owner_created_idx
    ON sonata.provider_usage(owner_id, created_at DESC, id);

CREATE TABLE sonata.outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text REFERENCES sonata.users(id) ON DELETE CASCADE,
    aggregate_type text NOT NULL CHECK (btrim(aggregate_type) <> ''),
    aggregate_id text NOT NULL CHECK (btrim(aggregate_id) <> ''),
    event_type text NOT NULL CHECK (btrim(event_type) <> ''),
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'dead')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    processed_at timestamptz,
    last_error_code text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_events_pending_idx
    ON sonata.outbox_events(status, available_at, created_at, id)
    WHERE status IN ('pending', 'failed');

-- +goose Down

DROP TABLE IF EXISTS sonata.outbox_events;
DROP TABLE IF EXISTS sonata.provider_usage;
DROP TABLE IF EXISTS sonata.documents;
DROP TABLE IF EXISTS sonata.memory_items;
DROP TABLE IF EXISTS sonata.affective_events;
DROP TABLE IF EXISTS sonata.affective_states;
DROP TABLE IF EXISTS sonata.user_manifests;
DROP TABLE IF EXISTS sonata.manifest_versions;
DROP TABLE IF EXISTS sonata.instruction_versions;
DROP TABLE IF EXISTS sonata.tool_calls;
DROP TABLE IF EXISTS sonata.role_runs;
DROP TABLE IF EXISTS sonata.cognitive_runs;
DROP TABLE IF EXISTS sonata.messages;
DROP TABLE IF EXISTS sonata.conversations;
DROP TABLE IF EXISTS sonata.users;
