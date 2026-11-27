-- MarkUpdateProcessed returns 0 rows affected when the update was already handled.
-- name: MarkUpdateProcessed :execrows
INSERT INTO processed_updates (update_id)
VALUES ($1)
ON CONFLICT (update_id) DO NOTHING;

-- UnmarkUpdate releases an update whose handling failed, so a redelivery is retried.
-- name: UnmarkUpdate :exec
DELETE FROM processed_updates
WHERE update_id = $1;
