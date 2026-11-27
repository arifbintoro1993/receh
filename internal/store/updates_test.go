package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// tagDB is a DBTX whose Exec returns a fixed command tag or error.
type tagDB struct {
	tag  string
	err  error
	args []any
}

func (d *tagDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	d.args = args
	return pgconn.NewCommandTag(d.tag), d.err
}

func (d *tagDB) Query(context.Context, string, ...any) (pgx.Rows, error) { panic("not used") }
func (d *tagDB) QueryRow(context.Context, string, ...any) pgx.Row        { panic("not used") }

func TestMarkUpdateProcessed(t *testing.T) {
	tests := []struct {
		name      string
		db        tagDB
		wantFirst bool
		wantErr   error
	}{
		{name: "first delivery", db: tagDB{tag: "INSERT 0 1"}, wantFirst: true},
		{name: "redelivery", db: tagDB{tag: "INSERT 0 0"}},
		{name: "fails", db: tagDB{err: errDB}, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first, err := New(&tt.db).MarkUpdateProcessed(context.Background(), 1001)
			if !errors.Is(err, tt.wantErr) || first != tt.wantFirst {
				t.Errorf("got %v, %v; want %v, %v", first, err, tt.wantFirst, tt.wantErr)
			}
			if !hasArg(tt.db.args, int64(1001)) {
				t.Errorf("args = %v", tt.db.args)
			}
		})
	}
}

func TestUnmarkUpdate(t *testing.T) {
	tests := []struct {
		name    string
		db      tagDB
		wantErr error
	}{
		{name: "ok", db: tagDB{tag: "DELETE 1"}},
		{name: "fails", db: tagDB{err: errDB}, wantErr: errDB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(&tt.db).UnmarkUpdate(context.Background(), 1001)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if !hasArg(tt.db.args, int64(1001)) {
				t.Errorf("args = %v", tt.db.args)
			}
		})
	}
}
