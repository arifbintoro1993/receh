-- MarkUpdateProcessed returns 0 rows affected when the update was already handled.
-- name: MarkUpdateProcessed :execrows
INSERT INTO processed_updates (update_id)
VALUES ($1)
ON CONFLICT (update_id) DO NOTHING;
