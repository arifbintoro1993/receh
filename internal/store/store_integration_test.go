//go:build integration

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/arifbintoro1993/receh/internal/store/db"
)

const chatID int64 = 42

func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("receh"),
		postgres.WithUsername("receh"),
		postgres.WithPassword("receh"),
		postgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	return url
}

func TestStore(t *testing.T) {
	ctx := context.Background()
	url := startPostgres(t)

	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// Twice: migrating an up-to-date database must be a no-op.
	for range 2 {
		if err := Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}

	q := db.New(pool)
	if err := q.EnsureUser(ctx, db.EnsureUserParams{
		ChatID: chatID, Tz: "Asia/Jakarta", CycleStartDay: 25,
		ReportTypes: []string{"weekly", "monthly"}, ReportHour: 20,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("ensure user is idempotent", func(t *testing.T) {
		err := q.EnsureUser(ctx, db.EnsureUserParams{
			ChatID: chatID, Tz: "UTC", CycleStartDay: 1, ReportTypes: []string{"cycle"}, ReportHour: 8,
		})
		if err != nil {
			t.Fatal(err)
		}
		var tz string
		if err := pool.QueryRow(ctx, "SELECT tz FROM users WHERE chat_id = $1", chatID).Scan(&tz); err != nil {
			t.Fatal(err)
		}
		if tz != "Asia/Jakarta" {
			t.Errorf("tz = %q, existing user must not be overwritten", tz)
		}
	})

	t.Run("update dedupe", func(t *testing.T) {
		for i, want := range []int64{1, 0} {
			n, err := q.MarkUpdateProcessed(ctx, 1001)
			if err != nil {
				t.Fatal(err)
			}
			if n != want {
				t.Errorf("call %d: rows = %d, want %d", i+1, n, want)
			}
		}
	})

	t.Run("category upsert is case-insensitive", func(t *testing.T) {
		a, err := q.UpsertCategory(ctx, db.UpsertCategoryParams{ChatID: chatID, Name: "Food"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := q.UpsertCategory(ctx, db.UpsertCategoryParams{ChatID: chatID, Name: "food"})
		if err != nil {
			t.Fatal(err)
		}
		if a.ID != b.ID || b.Name != "Food" {
			t.Errorf("got %+v then %+v, want same id and original name", a, b)
		}
	})

	t.Run("amount check rejects zero", func(t *testing.T) {
		cat, err := q.UpsertCategory(ctx, db.UpsertCategoryParams{ChatID: chatID, Name: "Other"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = q.InsertExpense(ctx, db.InsertExpenseParams{
			ChatID: chatID, Amount: 0, CategoryID: cat.ID, SpentAt: time.Now(),
		})
		if err == nil {
			t.Error("want check violation for amount 0")
		}
	})

	t.Run("period list is half-open and skips deleted", func(t *testing.T) {
		jkt, err := time.LoadLocation("Asia/Jakarta")
		if err != nil {
			t.Fatal(err)
		}
		start := time.Date(2026, 11, 1, 0, 0, 0, 0, jkt)
		end := time.Date(2026, 12, 1, 0, 0, 0, 0, jkt)
		cat, err := q.UpsertCategory(ctx, db.UpsertCategoryParams{ChatID: chatID, Name: "Transport"})
		if err != nil {
			t.Fatal(err)
		}

		insert := func(at time.Time, amount int64) int64 {
			id, err := q.InsertExpense(ctx, db.InsertExpenseParams{
				ChatID: chatID, Amount: amount, Note: "gojek", CategoryID: cat.ID, SpentAt: at,
			})
			if err != nil {
				t.Fatal(err)
			}
			return id
		}
		insert(start.Add(-time.Second), 1) // before
		first := insert(start, 10_000)     // first instant, included
		last := insert(end.Add(-time.Second), 20_000)
		insert(end, 2) // end is exclusive
		deleted := insert(start.Add(time.Hour), 3)
		if _, err := pool.Exec(ctx, "UPDATE expenses SET deleted_at = now() WHERE id = $1", deleted); err != nil {
			t.Fatal(err)
		}

		rows, err := q.ListExpensesInPeriod(ctx, db.ListExpensesInPeriodParams{ChatID: chatID, StartAt: start, EndAt: end})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].ID != first || rows[1].ID != last {
			t.Fatalf("rows = %+v, want ids [%d %d]", rows, first, last)
		}
		if rows[0].Category != "Transport" || rows[0].Amount != 10_000 {
			t.Errorf("row = %+v", rows[0])
		}
	})

	t.Run("period query uses partial index", func(t *testing.T) {
		rec := &sqlRecorder{}
		single := singleConnPool(t, url, rec)
		// Tiny tables make seq scans cheapest; disable them to prove the index
		// matches the query's predicate.
		if _, err := single.Exec(ctx, "SET enable_seqscan = off"); err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		if _, err := db.New(single).ListExpensesInPeriod(ctx, db.ListExpensesInPeriodParams{
			ChatID: chatID, StartAt: now, EndAt: now,
		}); err != nil {
			t.Fatal(err)
		}

		plan := explain(t, single, rec.sql, rec.args)
		if !strings.Contains(plan, "expenses_period_idx") {
			t.Errorf("plan does not use expenses_period_idx:\n%s", plan)
		}
	})
}

// sqlRecorder captures the last query sent, so EXPLAIN runs on the exact
// sqlc-generated SQL instead of a copy that could drift.
type sqlRecorder struct {
	sql  string
	args []any
}

func (r *sqlRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	r.sql, r.args = d.SQL, d.Args
	return ctx
}

func (r *sqlRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// singleConnPool keeps session settings like enable_seqscan on one connection.
func singleConnPool(t *testing.T, url string, tracer pgx.QueryTracer) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func explain(t *testing.T, pool *pgxpool.Pool, sql string, args []any) string {
	t.Helper()
	rows, err := pool.Query(context.Background(), "EXPLAIN "+sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan.String()
}
