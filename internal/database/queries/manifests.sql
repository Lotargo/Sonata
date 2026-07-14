-- name: LockUserManifestScope :exec
SELECT pg_advisory_xact_lock(
    hashtextextended(
        sqlc.arg(owner_id) || E'\x1f' || sqlc.arg(scope) || E'\x1f' || sqlc.arg(scope_id),
        0
    )
);

-- name: NewManifestID :one
SELECT gen_random_uuid()::text AS manifest_id;

-- name: GetUserManifestForUpdate :one
SELECT owner_id, scope, scope_id, manifest_id, version, status, content, content_hash, created_at, updated_at
FROM sonata.user_manifests
WHERE owner_id = sqlc.arg(owner_id)
  AND scope = sqlc.arg(scope)
  AND scope_id = sqlc.arg(scope_id)
FOR UPDATE;

-- name: InsertManifestVersion :one
INSERT INTO sonata.manifest_versions (
    manifest_id,
    version,
    owner_id,
    source,
    content_hash,
    metadata,
    created_at
)
VALUES (
    sqlc.arg(manifest_id),
    sqlc.arg(version),
    sqlc.narg(owner_id),
    sqlc.arg(source),
    sqlc.arg(content_hash),
    sqlc.arg(metadata)::jsonb,
    sqlc.arg(created_at)
)
RETURNING manifest_id, version, owner_id, source, content_hash, metadata, created_at;

-- name: UpsertUserManifest :one
INSERT INTO sonata.user_manifests (
    owner_id,
    scope,
    scope_id,
    manifest_id,
    version,
    status,
    content,
    content_hash,
    created_at,
    updated_at
)
VALUES (
    sqlc.arg(owner_id),
    sqlc.arg(scope),
    sqlc.arg(scope_id),
    sqlc.arg(manifest_id),
    sqlc.arg(version),
    sqlc.arg(status),
    sqlc.arg(content),
    sqlc.arg(content_hash),
    sqlc.arg(created_at),
    sqlc.arg(updated_at)
)
ON CONFLICT (owner_id, scope, scope_id) DO UPDATE
SET manifest_id = EXCLUDED.manifest_id,
    version = EXCLUDED.version,
    status = EXCLUDED.status,
    content = EXCLUDED.content,
    content_hash = EXCLUDED.content_hash,
    updated_at = EXCLUDED.updated_at
RETURNING owner_id, scope, scope_id, manifest_id, version, status, content, content_hash, created_at, updated_at;

-- name: GetUserManifest :one
SELECT owner_id, scope, scope_id, manifest_id, version, status, content, content_hash, created_at, updated_at
FROM sonata.user_manifests
WHERE owner_id = sqlc.arg(owner_id)
  AND scope = sqlc.arg(scope)
  AND scope_id = sqlc.arg(scope_id);

-- name: DeleteUserManifest :one
UPDATE sonata.user_manifests
SET status = 'deleted',
    content = '',
    updated_at = sqlc.arg(updated_at)
WHERE owner_id = sqlc.arg(owner_id)
  AND scope = sqlc.arg(scope)
  AND scope_id = sqlc.arg(scope_id)
RETURNING owner_id, scope, scope_id, manifest_id, version, status, content, content_hash, created_at, updated_at;
