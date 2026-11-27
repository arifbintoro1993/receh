-- name: GetUser :one
SELECT chat_id, tz, cycle_start_day, report_types, report_hour
FROM users
WHERE chat_id = $1;

-- CreateUserWithSeed inserts the user and, only if it is new, its categories
-- and keyword map, in one atomic statement. A concurrent first message for
-- the same chat makes the user insert a no-op, so nothing is seeded twice.
-- keyword_categories[i] is the category name for keywords[i].
-- name: CreateUserWithSeed :exec
WITH new_user AS (
    INSERT INTO users (chat_id, tz, cycle_start_day, report_types, report_hour)
    VALUES (@chat_id, @tz, @cycle_start_day, @report_types, @report_hour)
    ON CONFLICT (chat_id) DO NOTHING
    RETURNING chat_id
), new_categories AS (
    INSERT INTO categories (chat_id, name)
    SELECT new_user.chat_id, n.name
    FROM new_user, unnest(@category_names::text[]) AS n(name)
    RETURNING id, chat_id, name
)
INSERT INTO keyword_map (chat_id, keyword, category_id)
SELECT c.chat_id, k.keyword, c.id
FROM (
    -- Two set-returning calls in one SELECT list are zipped element-wise.
    SELECT unnest(@keywords::text[]) AS keyword,
           unnest(@keyword_categories::text[]) AS category
) k
JOIN new_categories c ON lower(c.name) = lower(k.category);
