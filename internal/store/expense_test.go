package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/arifbintoro1993/receh/internal/expense"
)

var errDB = errors.New("db down")

// reply is one scripted answer from scriptDB: rows for QueryRow, or err.
type reply struct {
	row []any
	err error
}

// scriptDB is a DBTX that answers calls in order and records their args.
// It checks the Go side (mapping, call sequence, error wrapping); SQL
// semantics are covered by the integration tests.
type scriptDB struct {
	t       *testing.T
	replies []reply
	args    [][]any
}

func (s *scriptDB) next(args []any) reply {
	s.t.Helper()
	s.args = append(s.args, args)
	if len(s.replies) == 0 {
		s.t.Fatalf("unexpected call #%d", len(s.args))
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r
}

func (s *scriptDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("INSERT 0 1"), s.next(args).err
}

func (s *scriptDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	s.t.Fatal("Query not expected")
	return nil, nil
}

func (s *scriptDB) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	r := s.next(args)
	return fakeRow(r)
}

func (s *scriptDB) done() {
	s.t.Helper()
	if len(s.replies) != 0 {
		s.t.Errorf("%d scripted replies not consumed", len(s.replies))
	}
}

type fakeRow reply

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(r.row) != len(dest) {
		return fmt.Errorf("scan: %d values into %d targets", len(r.row), len(dest))
	}
	for i, v := range r.row {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}
	return nil
}

func hasArg(args []any, want any) bool {
	return slices.ContainsFunc(args, func(a any) bool { return reflect.DeepEqual(a, want) })
}

func TestEnsureUser(t *testing.T) {
	newUser := expense.User{ChatID: 42, TZ: "Asia/Jakarta", CycleStartDay: 25, ReportTypes: []string{"weekly"}, ReportHour: 20}
	userRow := func(tz string) []any {
		return []any{int64(42), tz, int16(25), []string{"weekly"}, int16(20)}
	}
	seed := expense.Seed{
		Categories: []string{"Food", "Transport"},
		Keywords:   map[string]string{"kopi": "Food", "gojek": "Transport", "bakso": "Food"},
		Fallback:   "Food",
	}

	tests := []struct {
		name    string
		replies []reply
		want    expense.User
		wantErr error
	}{
		{
			name:    "existing user is one read",
			replies: []reply{{row: userRow("UTC")}},
			want:    expense.User{ChatID: 42, TZ: "UTC", CycleStartDay: 25, ReportTypes: []string{"weekly"}, ReportHour: 20},
		},
		{
			name:    "new user is seeded then re-read",
			replies: []reply{{err: pgx.ErrNoRows}, {}, {row: userRow("Asia/Jakarta")}},
			want:    newUser,
		},
		{name: "read fails", replies: []reply{{err: errDB}}, wantErr: errDB},
		{name: "create fails", replies: []reply{{err: pgx.ErrNoRows}, {err: errDB}}, wantErr: errDB},
		{name: "re-read fails", replies: []reply{{err: pgx.ErrNoRows}, {}, {err: errDB}}, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdb := &scriptDB{t: t, replies: tt.replies}
			got, err := New(sdb).EnsureUser(context.Background(), newUser, seed)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			sdb.done()
		})
	}

	t.Run("seed args are sorted and aligned", func(t *testing.T) {
		sdb := &scriptDB{t: t, replies: []reply{{err: pgx.ErrNoRows}, {}, {row: userRow("Asia/Jakarta")}}}
		if _, err := New(sdb).EnsureUser(context.Background(), newUser, seed); err != nil {
			t.Fatal(err)
		}
		create := sdb.args[1]
		for _, want := range []any{
			[]string{"bakso", "gojek", "kopi"},    // keywords, sorted
			[]string{"Food", "Transport", "Food"}, // their categories, same order
			[]string{"Food", "Transport"},         // category names
			int64(42), "Asia/Jakarta", int16(25), int16(20),
		} {
			if !hasArg(create, want) {
				t.Errorf("create args %v missing %v", create, want)
			}
		}
	})
}

func TestUpsertCategory(t *testing.T) {
	tests := []struct {
		name    string
		reply   reply
		want    expense.Category
		wantErr error
	}{
		{name: "ok", reply: reply{row: []any{int64(7), "Food"}}, want: expense.Category{ID: 7, Name: "Food"}},
		{name: "fails", reply: reply{err: errDB}, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdb := &scriptDB{t: t, replies: []reply{tt.reply}}
			got, err := New(sdb).UpsertCategory(context.Background(), 42, "food")
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("got %+v, %v; want %+v, %v", got, err, tt.want, tt.wantErr)
			}
			if !hasArg(sdb.args[0], "food") || !hasArg(sdb.args[0], int64(42)) {
				t.Errorf("args = %v", sdb.args[0])
			}
		})
	}
}

func TestCategoryByKeywords(t *testing.T) {
	tests := []struct {
		name      string
		reply     reply
		want      expense.Category
		wantFound bool
		wantErr   error
	}{
		{name: "found", reply: reply{row: []any{int64(7), "Food"}}, want: expense.Category{ID: 7, Name: "Food"}, wantFound: true},
		{name: "no mapping", reply: reply{err: pgx.ErrNoRows}},
		{name: "fails", reply: reply{err: errDB}, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdb := &scriptDB{t: t, replies: []reply{tt.reply}}
			got, found, err := New(sdb).CategoryByKeywords(context.Background(), 42, []string{"makan", "siang"})
			if !errors.Is(err, tt.wantErr) || got != tt.want || found != tt.wantFound {
				t.Errorf("got %+v, %v, %v; want %+v, %v, %v", got, found, err, tt.want, tt.wantFound, tt.wantErr)
			}
			if !hasArg(sdb.args[0], []string{"makan", "siang"}) {
				t.Errorf("args = %v", sdb.args[0])
			}
		})
	}
}

func TestInsertExpense(t *testing.T) {
	at := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	e := expense.Expense{ChatID: 42, Amount: 25_000, Note: "kopi", Category: expense.Category{ID: 7, Name: "Food"}, SpentAt: at}

	tests := []struct {
		name    string
		reply   reply
		want    int64
		wantErr error
	}{
		{name: "ok", reply: reply{row: []any{int64(99)}}, want: 99},
		{name: "fails", reply: reply{err: errDB}, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdb := &scriptDB{t: t, replies: []reply{tt.reply}}
			got, err := New(sdb).InsertExpense(context.Background(), e)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("got %d, %v; want %d, %v", got, err, tt.want, tt.wantErr)
			}
			for _, want := range []any{int64(42), int64(25_000), "kopi", int64(7), at} {
				if !hasArg(sdb.args[0], want) {
					t.Errorf("args %v missing %v", sdb.args[0], want)
				}
			}
		})
	}
}
