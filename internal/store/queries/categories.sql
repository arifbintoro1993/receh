-- UpsertCategory returns the existing category on a case-insensitive name match.
-- name: UpsertCategory :one
INSERT INTO categories (chat_id, name)
VALUES ($1, $2)
ON CONFLICT (chat_id, lower(name)) DO UPDATE SET name = categories.name
RETURNING id, name;
