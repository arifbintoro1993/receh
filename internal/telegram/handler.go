// Package telegram is the only package that talks to the Bot API. Handler
// holds the update logic and is testable without Telegram; Client adapts
// go-telegram/bot to it and implements Notifier.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
	"github.com/arifbintoro1993/receh/internal/parser"
	"github.com/arifbintoro1993/receh/internal/period"
	"github.com/arifbintoro1993/receh/internal/report"
)

// Notifier sends messages to a chat. It is the seam forks use to swap the
// messaging backend.
type Notifier interface {
	// SendText sends text to chatID, as a reply to message replyTo when non-zero.
	SendText(ctx context.Context, chatID int64, text string, replyTo int) (messageID int, err error)
}

// Update is the part of a Telegram update the bot acts on.
type Update struct {
	ID        int64
	ChatID    int64
	MessageID int
	Text      string
	SentAt    time.Time
}

// Deps are the handler's collaborators. Functions rather than interfaces keep
// the handler testable without adding seams.
type Deps struct {
	AllowedChatIDs  []int64
	Log             func(ctx context.Context, chatID int64, text string, at time.Time) (expense.Expense, error)
	MarkProcessed   func(ctx context.Context, updateID int64) (first bool, err error)
	UnmarkProcessed func(ctx context.Context, updateID int64) error
	Summarize       func(ctx context.Context, chatID int64, span expense.Span, at time.Time) (period.Period, report.Summary, error)
	Last            func(ctx context.Context, chatID int64, n int) ([]expense.Expense, error)
	Notifier        Notifier
	Messages        i18n.Translator
	Logger          *slog.Logger
}

type Handler struct {
	d       Deps
	allowed map[int64]bool

	mu        sync.Mutex
	strangers map[int64]bool // non-allowlisted chats already logged
}

func NewHandler(d Deps) *Handler {
	allowed := make(map[int64]bool, len(d.AllowedChatIDs))
	for _, id := range d.AllowedChatIDs {
		allowed[id] = true
	}
	return &Handler{d: d, allowed: allowed, strangers: map[int64]bool{}}
}

// Handle processes one update. Safe for concurrent use. Message text is never
// logged: it carries notes and amounts.
func (h *Handler) Handle(ctx context.Context, u Update) {
	if !h.allowed[u.ChatID] {
		h.logStranger(u.ChatID)
		return
	}
	if strings.TrimSpace(u.Text) == "" {
		return // stickers, photos, joins: nothing to do
	}

	// Mark first so a redelivered update can never log twice.
	first, err := h.d.MarkProcessed(ctx, u.ID)
	if err != nil {
		h.d.Logger.Error("mark update processed", "update_id", u.ID, "err", err)
		h.reply(ctx, u, h.d.Messages.T(i18n.InternalError))
		return
	}
	if !first {
		h.d.Logger.Debug("skip duplicate update", "update_id", u.ID)
		return
	}
	text, err := h.respond(ctx, u)
	if err != nil {
		h.d.Logger.Error("handle update", "update_id", u.ID, "err", err)
		// Release the update so a retry of the same delivery is not skipped.
		if err := h.d.UnmarkProcessed(ctx, u.ID); err != nil {
			h.d.Logger.Error("unmark update", "update_id", u.ID, "err", err)
		}
		text = h.d.Messages.T(i18n.InternalError)
	}
	h.reply(ctx, u, text)
}

// respond builds the reply. User mistakes become help replies; only internal
// failures are returned as errors.
func (h *Handler) respond(ctx context.Context, u Update) (string, error) {
	m := h.d.Messages
	if cmd, arg, ok := command(u.Text); ok {
		if span, label, ok := summarySpan(cmd); ok {
			p, s, err := h.d.Summarize(ctx, u.ChatID, span, u.SentAt)
			if err != nil {
				return "", fmt.Errorf("summarize %s: %w", span, err)
			}
			return formatSummary(m, label, p, s), nil
		}
		switch cmd {
		case "start", "help":
			return m.T(i18n.Usage), nil
		case "last":
			n, _ := strconv.Atoi(arg) // invalid or missing: the service uses its default
			expenses, err := h.d.Last(ctx, u.ChatID, n)
			if err != nil {
				return "", fmt.Errorf("last expenses: %w", err)
			}
			return formatLast(m, expenses), nil
		default:
			return m.T(i18n.UnknownCommand), nil
		}
	}

	e, err := h.d.Log(ctx, u.ChatID, u.Text, u.SentAt)
	switch {
	case err == nil:
		return formatLogged(m, e), nil
	case errors.Is(err, parser.ErrNoAmount):
		return m.T(i18n.Usage), nil
	case errors.Is(err, parser.ErrInvalidDate):
		return m.T(i18n.InvalidDate), nil
	}
	return "", fmt.Errorf("log expense: %w", err)
}

func summarySpan(cmd string) (expense.Span, i18n.Key, bool) {
	switch cmd {
	case "today":
		return expense.Today, i18n.SpanToday, true
	case "week":
		return expense.ThisWeek, i18n.SpanWeek, true
	case "month":
		return expense.ThisMonth, i18n.SpanMonth, true
	case "cycle":
		return expense.ThisCycle, i18n.SpanCycle, true
	}
	return "", 0, false
}

func (h *Handler) reply(ctx context.Context, u Update, text string) {
	if _, err := h.d.Notifier.SendText(ctx, u.ChatID, text, u.MessageID); err != nil {
		h.d.Logger.Error("send reply", "chat_id", u.ChatID, "err", err)
	}
}

// logStranger logs a non-allowlisted chat once so the owner can find their
// chat ID in the logs; the chat itself gets no reply.
func (h *Handler) logStranger(chatID int64) {
	h.mu.Lock()
	seen := h.strangers[chatID]
	h.strangers[chatID] = true
	h.mu.Unlock()
	if !seen {
		h.d.Logger.Info("ignoring chat not in ALLOWED_CHAT_IDS", "chat_id", chatID)
	}
}

// command splits a bot command into its lowercase name and trimmed argument
// ("/last@mybot 20" -> "last", "20").
func command(text string) (name, arg string, ok bool) {
	first, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	if !strings.HasPrefix(first, "/") || len(first) < 2 {
		return "", "", false
	}
	name, _, _ = strings.Cut(first[1:], "@")
	return strings.ToLower(name), strings.TrimSpace(rest), true
}
