// Package expense holds the domain logic for logging expenses. It owns the
// domain types and the Store interface it needs; package store implements it.
package expense

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/arifbintoro1993/receh/internal/parser"
)

type User struct {
	ChatID        int64
	TZ            string
	CycleStartDay int
	ReportTypes   []string
	ReportHour    int
}

type Category struct {
	ID   int64
	Name string
}

type Expense struct {
	ID       int64
	ChatID   int64
	Amount   int64
	Note     string
	Category Category
	SpentAt  time.Time
}

// Store is the persistence the expense domain needs.
type Store interface {
	// EnsureUser returns the chat's user, creating it from u and seeding its
	// categories and keywords atomically if it does not exist yet.
	EnsureUser(ctx context.Context, u User, seed Seed) (User, error)
	// UpsertCategory returns the category matching name case-insensitively, creating it if missing.
	UpsertCategory(ctx context.Context, chatID int64, name string) (Category, error)
	// CategoryByKeywords returns the category of the earliest keyword in keywords
	// that has a mapping; found is false if none has.
	CategoryByKeywords(ctx context.Context, chatID int64, keywords []string) (c Category, found bool, err error)
	InsertExpense(ctx context.Context, e Expense) (id int64, err error)
}

type Service struct {
	store    Store
	defaults User // ChatID ignored; settings for new chats
	seed     Seed
}

// NewService validates the seed up front so a bad config.yaml fails at startup.
func NewService(store Store, defaults User, seed Seed) (*Service, error) {
	if _, err := time.LoadLocation(defaults.TZ); err != nil {
		return nil, fmt.Errorf("default time zone: %w", err)
	}
	if err := seed.validate(); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	return &Service{store: store, defaults: defaults, seed: seed}, nil
}

// Log parses text and stores it as one expense. at is when the message was
// sent; dates are resolved in the chat's own time zone. Parse failures are
// returned as-is (parser.ErrNoAmount, parser.ErrInvalidDate) so the caller
// can reply with help.
func (s *Service) Log(ctx context.Context, chatID int64, text string, at time.Time) (Expense, error) {
	newUser := s.defaults
	newUser.ChatID = chatID
	user, err := s.store.EnsureUser(ctx, newUser, s.seed)
	if err != nil {
		return Expense{}, fmt.Errorf("ensure user: %w", err)
	}
	loc, err := time.LoadLocation(user.TZ)
	if err != nil {
		return Expense{}, fmt.Errorf("user time zone %q: %w", user.TZ, err)
	}

	entry, err := parser.Parse(text, at.In(loc))
	if err != nil {
		return Expense{}, err
	}

	category, err := s.categorize(ctx, chatID, entry)
	if err != nil {
		return Expense{}, fmt.Errorf("categorize: %w", err)
	}

	e := Expense{ChatID: chatID, Amount: entry.Amount, Note: entry.Note, Category: category, SpentAt: entry.SpentAt}
	if e.ID, err = s.store.InsertExpense(ctx, e); err != nil {
		return Expense{}, fmt.Errorf("insert expense: %w", err)
	}
	return e, nil
}

// categorize picks the category: an explicit #tag (created if unknown), else
// the first note word with a keyword mapping, else the fallback category.
func (s *Service) categorize(ctx context.Context, chatID int64, entry parser.Entry) (Category, error) {
	if entry.Tag != "" {
		return s.store.UpsertCategory(ctx, chatID, entry.Tag)
	}
	if words := strings.Fields(strings.ToLower(entry.Note)); len(words) > 0 {
		c, found, err := s.store.CategoryByKeywords(ctx, chatID, words)
		if err != nil || found {
			return c, err
		}
	}
	return s.store.UpsertCategory(ctx, chatID, s.seed.Fallback)
}
