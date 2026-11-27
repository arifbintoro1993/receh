package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpenErrors(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "invalid url", url: "postgres://%zz", wantErr: "parse database url"},
		// Port 1 refuses immediately, so this needs no database and stays fast.
		{name: "unreachable database", url: "postgres://u:p@127.0.0.1:1/receh?connect_timeout=1", wantErr: "ping database"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pool, err := Open(ctx, tt.url)
			if err == nil {
				pool.Close()
				t.Fatal("Open() err = nil, want error")
			}
			if pool != nil {
				t.Error("Open() returned a pool alongside an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Open() err = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestMigrateUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// pgxpool connects lazily, so this builds without a server and fails on first use.
	pool, err := pgxpool.New(ctx, "postgres://u:p@127.0.0.1:1/receh?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := Migrate(ctx, pool); err == nil {
		t.Fatal("Migrate() err = nil, want error")
	}
}
