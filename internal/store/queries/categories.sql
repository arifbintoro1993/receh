-- UpsertCategory returns the existing category on a case-insensitive name match.
-- name: UpsertCategory :one
INSERT INTO categories (chat_id, name)
VALUES ($1, $2)
ON CONFLICT (chat_id, lower(name)) DO UPDATE SET name = categories.name
RETURNING id, name;

-- CategoryByKeywords returns the category of the earliest keyword in the
-- array that has a mapping. PK lookups on keyword_map: O(w log n) for w words.
-- name: CategoryByKeywords :one
SELECT c.id, c.name
FROM keyword_map k
JOIN categories c ON c.id = k.category_id
WHERE k.chat_id = @chat_id
  AND k.keyword = ANY(@keywords::text[])
ORDER BY array_position(@keywords::text[], k.keyword)
LIMIT 1;
