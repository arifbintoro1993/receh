-- name: InsertExpense :one
INSERT INTO expenses (chat_id, amount, note, category_id, spent_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- Range scan on expenses_period_idx: O(log n + k).
-- name: ListExpensesInPeriod :many
SELECT e.id, e.amount, e.note, e.spent_at, c.name AS category
FROM expenses e
JOIN categories c ON c.id = e.category_id
WHERE e.chat_id = $1
  AND e.spent_at >= sqlc.arg(start_at)
  AND e.spent_at < sqlc.arg(end_at)
  AND e.deleted_at IS NULL
ORDER BY e.spent_at, e.id;

-- Backward scan on expenses_period_idx, stopping after n rows: O(log n + limit).
-- name: ListLastExpenses :many
SELECT e.id, e.amount, e.note, e.spent_at, c.name AS category
FROM expenses e
JOIN categories c ON c.id = e.category_id
WHERE e.chat_id = $1
  AND e.deleted_at IS NULL
ORDER BY e.spent_at DESC, e.id DESC
LIMIT sqlc.arg(n);
