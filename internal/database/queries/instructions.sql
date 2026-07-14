-- name: GetInstructionVersion :one
SELECT instruction_id, version, content_hash, metadata, created_at
FROM sonata.instruction_versions
WHERE instruction_id = sqlc.arg(instruction_id)
  AND version = sqlc.arg(version);

-- name: UpsertInstructionVersion :one
INSERT INTO sonata.instruction_versions (
    instruction_id,
    version,
    content_hash,
    metadata,
    created_at
)
VALUES (
    sqlc.arg(instruction_id),
    sqlc.arg(version),
    sqlc.arg(content_hash),
    sqlc.arg(metadata)::jsonb,
    sqlc.arg(created_at)
)
ON CONFLICT (instruction_id, version) DO UPDATE
SET content_hash = EXCLUDED.content_hash,
    metadata = EXCLUDED.metadata,
    created_at = EXCLUDED.created_at
RETURNING instruction_id, version, content_hash, metadata, created_at;
