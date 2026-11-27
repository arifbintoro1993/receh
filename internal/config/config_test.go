package config

import (
	"testing"
)

func TestLoad(t *testing.T) {
	valid := map[string]string{
		"TELEGRAM_TOKEN":   "123:abc",
		"ALLOWED_CHAT_IDS": "111,-222",
		"DATABASE_URL":     "postgres://u:p@localhost:5432/receh",
	}

	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
		check   func(t *testing.T, c Config)
	}{
		{
			name: "defaults applied",
			env:  valid,
			check: func(t *testing.T, c Config) {
				if c.TZ.String() != "Asia/Jakarta" {
					t.Errorf("TZ = %q, want Asia/Jakarta", c.TZ)
				}
				if c.Locale != "id" {
					t.Errorf("Locale = %q, want id", c.Locale)
				}
				if len(c.AllowedChatIDs) != 2 || c.AllowedChatIDs[0] != 111 || c.AllowedChatIDs[1] != -222 {
					t.Errorf("AllowedChatIDs = %v, want [111 -222]", c.AllowedChatIDs)
				}
			},
		},
		{
			name: "overrides",
			env:  with(valid, map[string]string{"TZ": "Asia/Makassar", "LOCALE": "en"}),
			check: func(t *testing.T, c Config) {
				if c.TZ.String() != "Asia/Makassar" || c.Locale != "en" {
					t.Errorf("got TZ=%q Locale=%q", c.TZ, c.Locale)
				}
			},
		},
		{name: "missing token", env: with(valid, map[string]string{"TELEGRAM_TOKEN": ""}), wantErr: true},
		{name: "missing allowlist", env: with(valid, map[string]string{"ALLOWED_CHAT_IDS": ""}), wantErr: true},
		{name: "missing database url", env: with(valid, map[string]string{"DATABASE_URL": ""}), wantErr: true},
		{name: "bad chat id", env: with(valid, map[string]string{"ALLOWED_CHAT_IDS": "abc"}), wantErr: true},
		{name: "bad time zone", env: with(valid, map[string]string{"TZ": "Mars/Base"}), wantErr: true},
		{name: "unsupported locale", env: with(valid, map[string]string{"LOCALE": "fr"}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Load(tt.env)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, c)
			}
		})
	}
}

func with(base, over map[string]string) map[string]string {
	m := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		m[k] = v
	}
	for k, v := range over {
		if v == "" {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	return m
}
