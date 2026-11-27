-- name: EnsureUser :exec
INSERT INTO users (chat_id, tz, cycle_start_day, report_types, report_hour)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (chat_id) DO NOTHING;
