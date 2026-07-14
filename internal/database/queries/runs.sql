-- name: CreateCognitiveRun :one
INSERT INTO sonata.cognitive_runs (
    owner_id,
    conversation_id,
    request_message_id,
    route,
    status,
    started_at,
    metadata
)
VALUES (
    sqlc.arg(owner_id),
    sqlc.arg(conversation_id),
    sqlc.arg(request_message_id),
    sqlc.arg(route),
    sqlc.arg(status),
    sqlc.arg(started_at),
    sqlc.arg(metadata)::jsonb
)
RETURNING id, owner_id, conversation_id, request_message_id, route, status, started_at, completed_at, metadata;

-- name: CreateRoleRun :one
INSERT INTO sonata.role_runs (
    owner_id,
    cognitive_run_id,
    phase,
    perspective,
    status,
    model_id,
    instruction_id,
    instruction_version,
    instruction_hash,
    manifest_id,
    manifest_version,
    manifest_hash,
    manifest_source,
    latency_ms,
    usage,
    error_code,
    created_at
)
VALUES (
    sqlc.arg(owner_id),
    sqlc.arg(cognitive_run_id),
    sqlc.arg(phase),
    sqlc.arg(perspective),
    sqlc.arg(status),
    sqlc.arg(model_id),
    sqlc.arg(instruction_id),
    sqlc.arg(instruction_version),
    sqlc.arg(instruction_hash),
    sqlc.arg(manifest_id),
    sqlc.arg(manifest_version),
    sqlc.arg(manifest_hash),
    sqlc.arg(manifest_source),
    sqlc.arg(latency_ms),
    sqlc.arg(usage)::jsonb,
    sqlc.arg(error_code),
    sqlc.arg(created_at)
)
RETURNING id, owner_id, cognitive_run_id, phase, perspective, status, model_id, instruction_id, instruction_version, instruction_hash, manifest_id, manifest_version, manifest_hash, manifest_source, latency_ms, usage, error_code, created_at;

-- name: CompleteRoleRun :one
UPDATE sonata.role_runs
SET status = sqlc.arg(status),
    model_id = sqlc.arg(model_id),
    latency_ms = sqlc.arg(latency_ms),
    usage = sqlc.arg(usage)::jsonb,
    error_code = sqlc.arg(error_code)
WHERE owner_id = sqlc.arg(owner_id)
  AND cognitive_run_id = sqlc.arg(cognitive_run_id)
  AND id = sqlc.arg(role_run_id)
RETURNING id, owner_id, cognitive_run_id, phase, perspective, status, model_id, instruction_id, instruction_version, instruction_hash, manifest_id, manifest_version, manifest_hash, manifest_source, latency_ms, usage, error_code, created_at;

-- name: CompleteCognitiveRun :one
UPDATE sonata.cognitive_runs
SET status = sqlc.arg(status),
    completed_at = sqlc.arg(completed_at),
    metadata = sqlc.arg(metadata)::jsonb
WHERE owner_id = sqlc.arg(owner_id)
  AND id = sqlc.arg(cognitive_run_id)
RETURNING id, owner_id, conversation_id, request_message_id, route, status, started_at, completed_at, metadata;

-- name: GetCognitiveRun :one
SELECT id, owner_id, conversation_id, request_message_id, route, status, started_at, completed_at, metadata
FROM sonata.cognitive_runs
WHERE owner_id = sqlc.arg(owner_id)
  AND id = sqlc.arg(cognitive_run_id);

-- name: ListRoleRuns :many
SELECT id, owner_id, cognitive_run_id, phase, perspective, status, model_id, instruction_id, instruction_version, instruction_hash, manifest_id, manifest_version, manifest_hash, manifest_source, latency_ms, usage, error_code, created_at
FROM sonata.role_runs
WHERE owner_id = sqlc.arg(owner_id)
  AND cognitive_run_id = sqlc.arg(cognitive_run_id)
ORDER BY created_at, id;
