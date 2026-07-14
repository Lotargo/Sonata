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
  AND status = 'RUNNING'
  AND phase = sqlc.arg(expected_phase)
  AND perspective = sqlc.arg(expected_perspective)
  AND instruction_id = sqlc.arg(expected_instruction_id)
  AND instruction_version = sqlc.arg(expected_instruction_version)
  AND instruction_hash = sqlc.arg(expected_instruction_hash)
  AND manifest_id = sqlc.arg(expected_manifest_id)
  AND manifest_version = sqlc.arg(expected_manifest_version)
  AND manifest_hash = sqlc.arg(expected_manifest_hash)
  AND manifest_source = sqlc.arg(expected_manifest_source)
RETURNING id, owner_id, cognitive_run_id, phase, perspective, status, model_id, instruction_id, instruction_version, instruction_hash, manifest_id, manifest_version, manifest_hash, manifest_source, latency_ms, usage, error_code, created_at;

-- name: CompleteCognitiveRun :one
UPDATE sonata.cognitive_runs AS run
SET status = sqlc.arg(status),
    completed_at = sqlc.arg(completed_at),
    metadata = sqlc.arg(metadata)::jsonb
WHERE run.owner_id = sqlc.arg(owner_id)
  AND run.id = sqlc.arg(cognitive_run_id)
  AND run.status = 'RUNNING'
  AND NOT EXISTS (
      SELECT 1
      FROM sonata.role_runs AS role
      WHERE role.owner_id = run.owner_id
        AND role.cognitive_run_id = run.id
        AND role.status = 'RUNNING'
  )
  AND (
      sqlc.arg(status) <> 'OK'
      OR NOT EXISTS (
          SELECT 1
          FROM sonata.role_runs AS role
          WHERE role.owner_id = run.owner_id
            AND role.cognitive_run_id = run.id
            AND role.status <> 'OK'
      )
  )
RETURNING run.id, run.owner_id, run.conversation_id, run.request_message_id, run.route, run.status, run.started_at, run.completed_at, run.metadata;

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
