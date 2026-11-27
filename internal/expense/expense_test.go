package expense

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/arifbintoro1993/receh/internal/parser"
)

const chatID int64 = 42

var errStore = errors.New("store down")

// fakeStore is an in-memory Store for one or more chats. Set an err* field
// to make that method fail.
type fakeStore struct {
	users      map[int64]User
	categories map[int64][]Category
	keywords   map[int64]map[string]int64 // keyword -> category id
	expenses   []Expense
	seeded     int
	nextID     int64

	errEnsure, errUpsert, errKeywords, errInsert error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:      map[int64]User{},
		categories: map[int64][]Category{},
		keywords:   map[int64]map[string]int64{},
	}
}

func (f *fakeStore) id() int64 { f.nextID++; return f.nextID }

func (f *fakeStore) EnsureUser(_ context.Context, u User, seed Seed) (User, error) {
	if f.errEnsure != nil {
		return User{}, f.errEnsure
	}
	if got, ok := f.users[u.ChatID]; ok {
		return got, nil
	}
	f.users[u.ChatID] = u
	f.seeded++
	f.keywords[u.ChatID] = map[string]int64{}
	for _, name := range seed.Categories {
		f.categories[u.ChatID] = append(f.categories[u.ChatID], Category{ID: f.id(), Name: name})
	}
	for kw, name := range seed.Keywords {
		c, _ := f.find(u.ChatID, name)
		f.keywords[u.ChatID][kw] = c.ID
	}
	return u, nil
}

func (f *fakeStore) find(chatID int64, name string) (Category, bool) {
	for _, c := range f.categories[chatID] {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Category{}, false
}

func (f *fakeStore) UpsertCategory(_ context.Context, chatID int64, name string) (Category, error) {
	if f.errUpsert != nil {
		return Category{}, f.errUpsert
	}
	if c, ok := f.find(chatID, name); ok {
		return c, nil
	}
	c := Category{ID: f.id(), Name: name}
	f.categories[chatID] = append(f.categories[chatID], c)
	return c, nil
}

func (f *fakeStore) CategoryByKeywords(_ context.Context, chatID int64, keywords []string) (Category, bool, error) {
	if f.errKeywords != nil {
		return Category{}, false, f.errKeywords
	}
	for _, kw := range keywords {
		if id, ok := f.keywords[chatID][kw]; ok {
			for _, c := range f.categories[chatID] {
				if c.ID == id {
					return c, true, nil
				}
			}
		}
	}
	return Category{}, false, nil
}

func (f *fakeStore) InsertExpense(_ context.Context, e Expense) (int64, error) {
	if f.errInsert != nil {
		return 0, f.errInsert
	}
	e.ID = f.id()
	f.expenses = append(f.expenses, e)
	return e.ID, nil
}

var defaults = User{TZ: "Asia/Jakarta", CycleStartDay: 25, ReportTypes: []string{"weekly", "monthly"}, ReportHour: 20}

func newService(t *testing.T, store Store) *Service {
	t.Helper()
	s, err := NewService(store, defaults, DefaultSeed())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLogCategorizes(t *testing.T) {
	at := time.Date(2026, 11, 28, 2, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		text         string
		wantCategory string
		wantNote     string
		wantAmount   int64
	}{
		{name: "tag matches existing case-insensitively", text: "listrik 1,2jt #BILLS", wantCategory: "Bills", wantNote: "listrik", wantAmount: 1_200_000},
		{name: "unknown tag creates category", text: "cilok 10k #jajan", wantCategory: "jajan", wantNote: "cilok", wantAmount: 10_000},
		{name: "tag beats keyword", text: "kopi 25k #Entertainment", wantCategory: "Entertainment", wantNote: "kopi", wantAmount: 25_000},
		{name: "keyword", text: "makan siang 45rb", wantCategory: "Food", wantNote: "makan siang", wantAmount: 45_000},
		{name: "keyword is case-insensitive", text: "Kopi 25k", wantCategory: "Food", wantNote: "Kopi", wantAmount: 25_000},
		{name: "earliest keyword wins", text: "grab beli kopi 30k", wantCategory: "Transport", wantNote: "grab beli kopi", wantAmount: 30_000},
		{name: "no keyword falls back", text: "buku 80k", wantCategory: "Other", wantNote: "buku", wantAmount: 80_000},
		{name: "amount only falls back", text: "25k", wantCategory: "Other", wantNote: "", wantAmount: 25_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			got, err := newService(t, store).Log(context.Background(), chatID, tt.text, at)
			if err != nil {
				t.Fatal(err)
			}
			if got.Category.Name != tt.wantCategory || got.Note != tt.wantNote || got.Amount != tt.wantAmount {
				t.Errorf("got %q/%q/%d, want %q/%q/%d", got.Category.Name, got.Note, got.Amount, tt.wantCategory, tt.wantNote, tt.wantAmount)
			}
			if len(store.expenses) != 1 || store.expenses[0].ID != got.ID || got.ID == 0 {
				t.Errorf("stored %+v, returned id %d", store.expenses, got.ID)
			}
			if got.ChatID != chatID || !got.SpentAt.Equal(at) {
				t.Errorf("chat/time = %d/%v, want %d/%v", got.ChatID, got.SpentAt, chatID, at)
			}
		})
	}
}

func TestLogSeedsNewUserOnce(t *testing.T) {
	store := newFakeStore()
	s := newService(t, store)
	for range 3 {
		if _, err := s.Log(context.Background(), chatID, "kopi 25k", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if store.seeded != 1 {
		t.Errorf("seeded %d times, want 1", store.seeded)
	}
	if got := store.users[chatID]; got.TZ != defaults.TZ || got.CycleStartDay != 25 || got.ReportHour != 20 {
		t.Errorf("new user = %+v, want instance defaults", got)
	}
}

func TestLogResolvesDatesInUserTimeZone(t *testing.T) {
	// 20:00 UTC on 30 Nov is already 1 Dec 03:00 in Jakarta, but still 30 Nov in UTC.
	at := time.Date(2026, 11, 30, 20, 0, 0, 0, time.UTC)
	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		userTZ string // pre-existing user's zone; empty means a new user on defaults
		want   time.Time
	}{
		{name: "new user uses default zone", want: time.Date(2026, 11, 30, 12, 0, 0, 0, jkt)},
		{name: "existing user keeps own zone", userTZ: "UTC", want: time.Date(2026, 11, 29, 12, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			if tt.userTZ != "" {
				u := defaults
				u.ChatID, u.TZ = chatID, tt.userTZ
				if _, err := store.EnsureUser(context.Background(), u, DefaultSeed()); err != nil {
					t.Fatal(err)
				}
			}
			got, err := newService(t, store).Log(context.Background(), chatID, "kopi 25k kemarin", at)
			if err != nil {
				t.Fatal(err)
			}
			if !got.SpentAt.Equal(tt.want) {
				t.Errorf("SpentAt = %v, want %v", got.SpentAt, tt.want)
			}
		})
	}
}

func TestLogErrors(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		setup   func(f *fakeStore)
		wantErr error
	}{
		{name: "unparseable text", text: "halo bot", wantErr: parser.ErrNoAmount},
		{name: "invalid date", text: "kopi 25k @2026-13-01", wantErr: parser.ErrInvalidDate},
		{name: "ensure user fails", text: "kopi 25k", setup: func(f *fakeStore) { f.errEnsure = errStore }, wantErr: errStore},
		{name: "tag upsert fails", text: "kopi 25k #x", setup: func(f *fakeStore) { f.errUpsert = errStore }, wantErr: errStore},
		{name: "keyword lookup fails", text: "kopi 25k", setup: func(f *fakeStore) { f.errKeywords = errStore }, wantErr: errStore},
		{name: "fallback upsert fails", text: "buku 25k", setup: func(f *fakeStore) { f.errUpsert = errStore }, wantErr: errStore},
		{name: "insert fails", text: "kopi 25k", setup: func(f *fakeStore) { f.errInsert = errStore }, wantErr: errStore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			if tt.setup != nil {
				tt.setup(store)
			}
			_, err := newService(t, store).Log(context.Background(), chatID, tt.text, time.Now())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if len(store.expenses) != 0 {
				t.Errorf("stored %d expenses on error, want 0", len(store.expenses))
			}
		})
	}

	t.Run("stored time zone is invalid", func(t *testing.T) {
		store := newFakeStore()
		store.users[chatID] = User{ChatID: chatID, TZ: "Mars/Base"}
		if _, err := newService(t, store).Log(context.Background(), chatID, "kopi 25k", time.Now()); err == nil {
			t.Fatal("err = nil, want time zone error")
		}
	})
}

func TestNewService(t *testing.T) {
	seed := func(edit func(s *Seed)) Seed {
		s := DefaultSeed()
		edit(&s)
		return s
	}

	tests := []struct {
		name     string
		defaults User
		seed     Seed
		wantErr  bool
	}{
		{name: "defaults are valid", defaults: defaults, seed: DefaultSeed()},
		{name: "bad default zone", defaults: User{TZ: "Mars/Base"}, seed: DefaultSeed(), wantErr: true},
		{name: "empty category", defaults: defaults, seed: seed(func(s *Seed) { s.Categories = append(s.Categories, "") }), wantErr: true},
		{name: "fallback not a category", defaults: defaults, seed: seed(func(s *Seed) { s.Fallback = "Misc" }), wantErr: true},
		{name: "fallback matches case-insensitively", defaults: defaults, seed: seed(func(s *Seed) { s.Fallback = "other" })},
		{name: "keyword to unknown category", defaults: defaults, seed: seed(func(s *Seed) { s.Keywords["buku"] = "Books" }), wantErr: true},
		{name: "uppercase keyword", defaults: defaults, seed: seed(func(s *Seed) { s.Keywords["Buku"] = "Shopping" }), wantErr: true},
		{name: "multi-word keyword", defaults: defaults, seed: seed(func(s *Seed) { s.Keywords["makan siang"] = "Food" }), wantErr: true},
		{name: "empty keyword", defaults: defaults, seed: seed(func(s *Seed) { s.Keywords[""] = "Food" }), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(newFakeStore(), tt.defaults, tt.seed)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
