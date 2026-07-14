-- name: InsertOutboxEvent :one
INSERT INTO sonata.outbox_events (
    owner_id,
    aggregate_type,
    aggregate_id,
    event_type,
    payload,
    status,
    attempts,
    available_at,
    created_at
)
VALUES (
    sqlc.narg(owner_id),
    sqlc.arg(aggregate_type),
    sqlc.arg(aggregate_id),
    sqlc.arg(event_type),
    sqlc.arg(payload)::jsonb,
    sqlc.arg(status),
    sqlc.arg(attempts),
    sqlc.arg(available_at),
    sqlc.arg(created_at)
)
RETURNING id, owner_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, available_at, locked_at, processed_at, last_error_code, created_at;

-- name: LockPendingOutboxEvents :many
UPDATE sonata.outbox_events AS outer_ev
SET status = 'processing',
    locked_at = sqlc.arg(locked_at)
WHERE outer_ev.id IN (
    SELECT inner_ev.id
    FROM sonata.outbox_events AS inner_ev
    WHERE inner_ev.status IN ('pending', 'failed')
      AND inner_ev.available_at <= sqlc.arg(now)
    ORDER BY inner_ev.available_at, inner_ev.created_at, inner_ev.id
    LIMIT sqlc.arg(limit_count)
    FOR UPDATE SKIP LOCKED
)
RETURNING id, owner_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, available_at, locked_at, processed_at, last_error_code, created_at;

-- name: CompleteOutboxEvent :one
UPDATE sonata.outbox_events
SET status = 'completed',
    processed_at = sqlc.arg(processed_at),
    attempts = attempts + 1
WHERE id = sqlc.arg(id)
  AND status = 'processing'
RETURNING id, status, attempts, processed_at;

-- name: FailOutboxEvent :one
UPDATE sonata.outbox_events
SET status = sqlc.arg(status),
    available_at = sqlc.arg(next_available_at),
    last_error_code = sqlc.arg(last_error_code),
    locked_at = NULL,
    attempts = attempts + 1
WHERE id = sqlc.arg(id)
  AND status = 'processing'
RETURNING id, status, attempts, available_at;
