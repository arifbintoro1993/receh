package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
	"github.com/arifbintoro1993/receh/internal/parser"
)

const (
	allowedChat int64 = 111
	strangeChat int64 = 999
)

var errBoom = errors.New("boom")

type sent struct {
	chatID  int64
	text    string
	replyTo int
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []sent
	err  error
}

func (n *fakeNotifier) SendText(_ context.Context, chatID int64, text string, replyTo int) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, sent{chatID, text, replyTo})
	return len(n.sent), n.err
}

// harness wires a Handler to in-memory fakes and records what happened.
type harness struct {
	h         *Handler
	notifier  *fakeNotifier
	logs      *bytes.Buffer
	processed map[int64]bool
	logCalls  []string

	logResult  expense.Expense
	logErr     error
	markErr    error
	unmarkErr  error
	lastLogged time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	messages, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	hs := &harness{notifier: &fakeNotifier{}, logs: &bytes.Buffer{}, processed: map[int64]bool{}}
	hs.h = NewHandler(Deps{
		AllowedChatIDs: []int64{allowedChat},
		Log: func(_ context.Context, chatID int64, text string, at time.Time) (expense.Expense, error) {
			hs.logCalls = append(hs.logCalls, fmt.Sprintf("%d:%s", chatID, text))
			hs.lastLogged = at
			return hs.logResult, hs.logErr
		},
		MarkProcessed: func(_ context.Context, id int64) (bool, error) {
			if hs.markErr != nil {
				return false, hs.markErr
			}
			if hs.processed[id] {
				return false, nil
			}
			hs.processed[id] = true
			return true, nil
		},
		UnmarkProcessed: func(_ context.Context, id int64) error {
			delete(hs.processed, id)
			return hs.unmarkErr
		},
		Notifier: hs.notifier,
		Messages: messages,
		Logger:   slog.New(slog.NewTextHandler(hs.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	return hs
}

func msg(id int64, chatID int64, text string) Update {
	return Update{ID: id, ChatID: chatID, MessageID: int(id) * 10, Text: text, SentAt: time.Unix(1_764_300_000, 0)}
}

func (hs *harness) replies() []string {
	out := make([]string, len(hs.notifier.sent))
	for i, s := range hs.notifier.sent {
		out[i] = s.text
	}
	return out
}

func TestHandleLogsExpense(t *testing.T) {
	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	spent := time.Date(2026, 11, 28, 9, 0, 0, 0, jkt)

	tests := []struct {
		name string
		note string
		want string
	}{
		{name: "with note", note: "kopi", want: "✅ Rp25.000 · Food\n📝 kopi\n📅 2026-11-28"},
		{name: "empty note shows dash", note: "", want: "✅ Rp25.000 · Food\n📝 —\n📅 2026-11-28"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarness(t)
			hs.logResult = expense.Expense{Amount: 25_000, Note: tt.note, Category: expense.Category{Name: "Food"}, SpentAt: spent}
			u := msg(1, allowedChat, "kopi 25k")

			hs.h.Handle(context.Background(), u)

			if len(hs.logCalls) != 1 || hs.logCalls[0] != "111:kopi 25k" {
				t.Errorf("log calls = %v", hs.logCalls)
			}
			if !hs.lastLogged.Equal(u.SentAt) {
				t.Errorf("logged at %v, want message time %v", hs.lastLogged, u.SentAt)
			}
			want := []sent{{chatID: allowedChat, text: tt.want, replyTo: u.MessageID}}
			if fmt.Sprint(hs.notifier.sent) != fmt.Sprint(want) {
				t.Errorf("sent %+v, want %+v", hs.notifier.sent, want)
			}
		})
	}
}

func TestHandleReplies(t *testing.T) {
	en, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		text         string
		logErr       error
		wantReply    string
		wantLogCalls int
		wantMarked   bool
	}{
		{name: "help", text: "/help", wantReply: en.T(i18n.Usage), wantMarked: true},
		{name: "start", text: "/start", wantReply: en.T(i18n.Usage), wantMarked: true},
		{name: "command with bot name and case", text: "/HELP@receh_bot now", wantReply: en.T(i18n.Usage), wantMarked: true},
		{name: "unknown command", text: "/today", wantReply: en.T(i18n.UnknownCommand), wantMarked: true},
		{name: "lone slash is text", text: "/", logErr: parser.ErrNoAmount, wantReply: en.T(i18n.Usage), wantLogCalls: 1, wantMarked: true},
		{name: "unparseable", text: "halo", logErr: parser.ErrNoAmount, wantReply: en.T(i18n.Usage), wantLogCalls: 1, wantMarked: true},
		{name: "invalid date", text: "kopi 25k @2026-13-01", logErr: fmt.Errorf("%w: x", parser.ErrInvalidDate), wantReply: en.T(i18n.InvalidDate), wantLogCalls: 1, wantMarked: true},
		{name: "internal error unmarks", text: "kopi 25k", logErr: errBoom, wantReply: en.T(i18n.InternalError), wantLogCalls: 1, wantMarked: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarness(t)
			hs.logErr = tt.logErr

			hs.h.Handle(context.Background(), msg(1, allowedChat, tt.text))

			if r := hs.replies(); len(r) != 1 || r[0] != tt.wantReply {
				t.Errorf("replies = %q, want [%q]", r, tt.wantReply)
			}
			if len(hs.logCalls) != tt.wantLogCalls {
				t.Errorf("log calls = %d, want %d", len(hs.logCalls), tt.wantLogCalls)
			}
			if hs.processed[1] != tt.wantMarked {
				t.Errorf("marked = %v, want %v", hs.processed[1], tt.wantMarked)
			}
		})
	}
}

func TestHandleSkipsDuplicateUpdate(t *testing.T) {
	hs := newHarness(t)
	u := msg(1, allowedChat, "kopi 25k")
	hs.h.Handle(context.Background(), u)
	hs.h.Handle(context.Background(), u)

	if len(hs.logCalls) != 1 || len(hs.notifier.sent) != 1 {
		t.Errorf("log calls %d, replies %d; want 1 and 1", len(hs.logCalls), len(hs.notifier.sent))
	}
}

func TestHandleRetriesAfterInternalError(t *testing.T) {
	hs := newHarness(t)
	u := msg(1, allowedChat, "kopi 25k")
	hs.logErr = errBoom
	hs.h.Handle(context.Background(), u)
	hs.logErr = nil
	hs.h.Handle(context.Background(), u) // redelivery must not be skipped

	if len(hs.logCalls) != 2 {
		t.Errorf("log calls = %d, want 2", len(hs.logCalls))
	}
}

func TestHandleIgnoresStrangers(t *testing.T) {
	hs := newHarness(t)
	for i := range 3 {
		hs.h.Handle(context.Background(), msg(int64(i+1), strangeChat, "kopi 25k"))
	}

	if len(hs.notifier.sent) != 0 || len(hs.logCalls) != 0 || len(hs.processed) != 0 {
		t.Errorf("stranger got replies %d, logs %d, marks %d; want none", len(hs.notifier.sent), len(hs.logCalls), len(hs.processed))
	}
	if n := strings.Count(hs.logs.String(), "chat_id=999"); n != 1 {
		t.Errorf("stranger chat id logged %d times, want once:\n%s", n, hs.logs)
	}
}

func TestHandleIgnoresEmptyText(t *testing.T) {
	hs := newHarness(t)
	hs.h.Handle(context.Background(), msg(1, allowedChat, "   "))
	if len(hs.notifier.sent) != 0 || len(hs.processed) != 0 {
		t.Error("empty message must be ignored")
	}
}

func TestHandleFailures(t *testing.T) {
	en, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("mark fails", func(t *testing.T) {
		hs := newHarness(t)
		hs.markErr = errBoom
		hs.h.Handle(context.Background(), msg(1, allowedChat, "kopi 25k"))
		if len(hs.logCalls) != 0 {
			t.Error("must not log an expense when dedupe fails")
		}
		if r := hs.replies(); len(r) != 1 || r[0] != en.T(i18n.InternalError) {
			t.Errorf("replies = %q", r)
		}
	})

	t.Run("unmark fails is logged", func(t *testing.T) {
		hs := newHarness(t)
		hs.logErr, hs.unmarkErr = errBoom, errBoom
		hs.h.Handle(context.Background(), msg(1, allowedChat, "kopi 25k"))
		if !strings.Contains(hs.logs.String(), "unmark update") {
			t.Errorf("logs = %s", hs.logs)
		}
	})

	t.Run("send fails is logged", func(t *testing.T) {
		hs := newHarness(t)
		hs.notifier.err = errBoom
		hs.h.Handle(context.Background(), msg(1, allowedChat, "/help"))
		if !strings.Contains(hs.logs.String(), "send reply") {
			t.Errorf("logs = %s", hs.logs)
		}
	})
}

func TestHandleNeverLogsMessageText(t *testing.T) {
	hs := newHarness(t)
	hs.logErr = errBoom // the error path logs the most
	hs.h.Handle(context.Background(), msg(1, allowedChat, "rahasia 777k"))
	hs.h.Handle(context.Background(), msg(2, strangeChat, "rahasia 777k"))

	if out := hs.logs.String(); strings.Contains(out, "rahasia") || strings.Contains(out, "777") {
		t.Errorf("logs leak message text:\n%s", out)
	}
}

func TestHandleConcurrentStrangers(t *testing.T) {
	hs := newHarness(t)
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { hs.h.Handle(context.Background(), msg(int64(i), strangeChat, "x")) })
	}
	wg.Wait()
	if n := strings.Count(hs.logs.String(), "chat_id=999"); n != 1 {
		t.Errorf("logged %d times, want once", n)
	}
}
