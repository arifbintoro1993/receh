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

	// Defaults for a chat's settings on first use; each chat can change them later.
	CycleStartDay int      `env:"CYCLE_START_DAY" envDefault:"25"`
	ReportTypes   []string `env:"REPORT_TYPES" envDefault:"weekly,monthly"`
	ReportHour    int      `env:"REPORT_HOUR" envDefault:"20"`
}

var (
	supportedLocales = map[string]bool{"id": true, "en": true}
	reportTypes      = map[string]bool{"weekly": true, "monthly": true, "cycle": true}
)

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
	if c.CycleStartDay < 1 || c.CycleStartDay > 31 {
		return Config{}, fmt.Errorf("CYCLE_START_DAY %d out of range 1..31", c.CycleStartDay)
	}
	if c.ReportHour < 0 || c.ReportHour > 23 {
		return Config{}, fmt.Errorf("REPORT_HOUR %d out of range 0..23", c.ReportHour)
	}
	for _, t := range c.ReportTypes {
		if !reportTypes[t] {
			return Config{}, fmt.Errorf("unknown REPORT_TYPES entry %q (want weekly, monthly or cycle)", t)
		}
	}
	return c, nil
}
