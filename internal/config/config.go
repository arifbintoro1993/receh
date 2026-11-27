// Package config is the single place environment variables are read (OSS-05).
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	TelegramToken  string         `env:"TELEGRAM_TOKEN,required,notEmpty"`
	AllowedChatIDs []int64        `env:"ALLOWED_CHAT_IDS,required,notEmpty"`
	DatabaseURL    string         `env:"DATABASE_URL,required,notEmpty"`
	TZ             *time.Location `env:"TZ" envDefault:"Asia/Jakarta"`
	Locale         string         `env:"LOCALE" envDefault:"id"`
	LogLevel       string         `env:"LOG_LEVEL" envDefault:"info"`
}

var supportedLocales = map[string]bool{"id": true, "en": true}

// Load parses config from the given environment, e.g. env.ToMap(os.Environ()).
// Taking a map instead of reading os.Environ keeps tests hermetic.
func Load(environ map[string]string) (Config, error) {
	var c Config
	if err := env.ParseWithOptions(&c, env.Options{Environment: environ}); err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}
	if !supportedLocales[c.Locale] {
		return Config{}, fmt.Errorf("unsupported LOCALE %q (want id or en)", c.Locale)
	}
	return c, nil
}
