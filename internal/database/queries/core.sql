-- name: EnsureUser :one
INSERT INTO sonata.users (id, updated_at)
VALUES (sqlc.arg(owner_id), sqlc.arg(updated_at))
ON CONFLICT (id) DO UPDATE
SET updated_at = GREATEST(sonata.users.updated_at, EXCLUDED.updated_at)
RETURNING id, created_at, updated_at;

-- name: UpsertConversation :one
INSERT INTO sonata.conversations (
    owner_id,
    id,
    title,
    created_at,
    updated_at
)
VALUES (
    sqlc.arg(owner_id),
    sqlc.arg(conversation_id),
    sqlc.arg(title),
    sqlc.arg(created_at),
    sqlc.arg(updated_at)
)
ON CONFLICT (owner_id, id) DO UPDATE
SET title = EXCLUDED.title,
    updated_at = GREATEST(sonata.conversations.updated_at, EXCLUDED.updated_at)
RETURNING owner_id, id, title, created_at, updated_at;

-- name: InsertMessage :one
INSERT INTO sonata.messages (
    owner_id,
    conversation_id,
    id,
    role,
    content,
    created_at
)
VALUES (
    sqlc.arg(owner_id),
    sqlc.arg(conversation_id),
    sqlc.arg(message_id),
    sqlc.arg(role),
    sqlc.arg(content)::jsonb,
    sqlc.arg(created_at)
)
RETURNING owner_id, conversation_id, id, role, content, created_at;

-- name: ListConversationMessages :many
SELECT owner_id, conversation_id, id, role, content, created_at
FROM sonata.messages
WHERE owner_id = sqlc.arg(owner_id)
  AND conversation_id = sqlc.arg(conversation_id)
ORDER BY created_at, id
LIMIT sqlc.arg(limit_count);
