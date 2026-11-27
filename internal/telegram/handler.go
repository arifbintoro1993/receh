// Package telegram is the only package that talks to the Bot API. Handler
// holds the update logic and is testable without Telegram; Client adapts
// go-telegram/bot to it and implements Notifier.
package telegram

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
	"github.com/arifbintoro1993/receh/internal/parser"
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
	h.reply(ctx, u, h.respond(ctx, u))
}

func (h *Handler) respond(ctx context.Context, u Update) string {
	m := h.d.Messages
	if cmd, ok := command(u.Text); ok {
		switch cmd {
		case "start", "help":
			return m.T(i18n.Usage)
		default:
			return m.T(i18n.UnknownCommand)
		}
	}

	e, err := h.d.Log(ctx, u.ChatID, u.Text, u.SentAt)
	switch {
	case err == nil:
		note := e.Note
		if note == "" {
			note = "—"
		}
		return m.T(i18n.Logged, i18n.Rupiah(e.Amount), e.Category.Name, note, e.SpentAt.Format(time.DateOnly))
	case errors.Is(err, parser.ErrNoAmount):
		return m.T(i18n.Usage)
	case errors.Is(err, parser.ErrInvalidDate):
		return m.T(i18n.InvalidDate)
	default:
		h.d.Logger.Error("log expense", "update_id", u.ID, "err", err)
		// Release the update so a retry of the same delivery is not skipped.
		if err := h.d.UnmarkProcessed(ctx, u.ID); err != nil {
			h.d.Logger.Error("unmark update", "update_id", u.ID, "err", err)
		}
		return m.T(i18n.InternalError)
	}
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

// command returns the lowercase name of a bot command ("/help@mybot" -> "help").
func command(text string) (string, bool) {
	first, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	if !strings.HasPrefix(first, "/") || len(first) < 2 {
		return "", false
	}
	name, _, _ := strings.Cut(first[1:], "@")
	return strings.ToLower(name), true
}
