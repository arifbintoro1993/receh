package store

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/store/db"
)

// Store implements expense.Store on top of the sqlc queries, mapping db rows
// to domain types so nothing outside this package sees sqlc types.
type Store struct {
	q *db.Queries
}

var _ expense.Store = (*Store)(nil)

// New takes a *pgxpool.Pool in production, or a transaction or fake in tests.
func New(dbtx db.DBTX) *Store {
	return &Store{q: db.New(dbtx)}
}

// EnsureUser is one query for an existing chat; a new chat costs two more
// (seed, then re-read, which also returns the winner of a concurrent create).
func (s *Store) EnsureUser(ctx context.Context, u expense.User, seed expense.Seed) (expense.User, error) {
	row, err := s.q.GetUser(ctx, u.ChatID)
	if err == nil {
		return toUser(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return expense.User{}, fmt.Errorf("get user: %w", err)
	}

	keywords := slices.Sorted(maps.Keys(seed.Keywords)) // sorted for deterministic args
	keywordCategories := make([]string, len(keywords))
	for i, kw := range keywords {
		keywordCategories[i] = seed.Keywords[kw]
	}
	if err := s.q.CreateUserWithSeed(ctx, db.CreateUserWithSeedParams{
		ChatID:            u.ChatID,
		Tz:                u.TZ,
		CycleStartDay:     int16(u.CycleStartDay),
		ReportTypes:       u.ReportTypes,
		ReportHour:        int16(u.ReportHour),
		CategoryNames:     seed.Categories,
		Keywords:          keywords,
		KeywordCategories: keywordCategories,
	}); err != nil {
		return expense.User{}, fmt.Errorf("create user: %w", err)
	}

	row, err = s.q.GetUser(ctx, u.ChatID)
	if err != nil {
		return expense.User{}, fmt.Errorf("get created user: %w", err)
	}
	return toUser(row), nil
}

func (s *Store) UpsertCategory(ctx context.Context, chatID int64, name string) (expense.Category, error) {
	row, err := s.q.UpsertCategory(ctx, db.UpsertCategoryParams{ChatID: chatID, Name: name})
	if err != nil {
		return expense.Category{}, fmt.Errorf("upsert category: %w", err)
	}
	return expense.Category{ID: row.ID, Name: row.Name}, nil
}

func (s *Store) CategoryByKeywords(ctx context.Context, chatID int64, keywords []string) (expense.Category, bool, error) {
	row, err := s.q.CategoryByKeywords(ctx, db.CategoryByKeywordsParams{ChatID: chatID, Keywords: keywords})
	if errors.Is(err, pgx.ErrNoRows) {
		return expense.Category{}, false, nil
	}
	if err != nil {
		return expense.Category{}, false, fmt.Errorf("category by keywords: %w", err)
	}
	return expense.Category{ID: row.ID, Name: row.Name}, true, nil
}

func (s *Store) InsertExpense(ctx context.Context, e expense.Expense) (int64, error) {
	id, err := s.q.InsertExpense(ctx, db.InsertExpenseParams{
		ChatID:     e.ChatID,
		Amount:     e.Amount,
		Note:       e.Note,
		CategoryID: e.Category.ID,
		SpentAt:    e.SpentAt,
	})
	if err != nil {
		return 0, fmt.Errorf("insert expense: %w", err)
	}
	return id, nil
}

func (s *Store) ExpensesInPeriod(ctx context.Context, chatID int64, start, end time.Time) ([]expense.Expense, error) {
	rows, err := s.q.ListExpensesInPeriod(ctx, db.ListExpensesInPeriodParams{ChatID: chatID, StartAt: start, EndAt: end})
	if err != nil {
		return nil, fmt.Errorf("list expenses in period: %w", err)
	}
	out := make([]expense.Expense, len(rows))
	for i, r := range rows {
		out[i] = toExpense(chatID, r.ID, r.Amount, r.Note, r.Category, r.SpentAt)
	}
	return out, nil
}

func (s *Store) LastExpenses(ctx context.Context, chatID int64, n int) ([]expense.Expense, error) {
	rows, err := s.q.ListLastExpenses(ctx, db.ListLastExpensesParams{ChatID: chatID, N: int32(n)})
	if err != nil {
		return nil, fmt.Errorf("list last expenses: %w", err)
	}
	out := make([]expense.Expense, len(rows))
	for i, r := range rows {
		out[i] = toExpense(chatID, r.ID, r.Amount, r.Note, r.Category, r.SpentAt)
	}
	return out, nil
}

func toExpense(chatID, id, amount int64, note, category string, spentAt time.Time) expense.Expense {
	return expense.Expense{
		ID: id, ChatID: chatID, Amount: amount, Note: note,
		Category: expense.Category{Name: category}, SpentAt: spentAt,
	}
}

func toUser(r db.GetUserRow) expense.User {
	return expense.User{
		ChatID:        r.ChatID,
		TZ:            r.Tz,
		CycleStartDay: int(r.CycleStartDay),
		ReportTypes:   r.ReportTypes,
		ReportHour:    int(r.ReportHour),
	}
}
