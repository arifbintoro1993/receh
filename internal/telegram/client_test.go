package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
)

const token = "123:test-token"

// fakeAPI is a minimal Bot API server: it serves queued updates once and
// records sendMessage calls.
type fakeAPI struct {
	mu        sync.Mutex
	updates   []models.Update
	failPolls int // number of getUpdates calls to fail
	sends     []map[string]string
	sendResp  string // raw JSON reply for sendMessage
	getMe     string // raw JSON reply for getMe
	sent      chan struct{}
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	t.Helper()
	api := &fakeAPI{
		sendResp: `{"ok":true,"result":{"message_id":77,"date":0,"chat":{"id":111,"type":"private"}}}`,
		getMe:    `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"receh","username":"receh_bot"}}`,
		sent:     make(chan struct{}, 10),
	}
	srv := httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(srv.Close)
	return api, srv
}

func (a *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	method := strings.TrimPrefix(r.URL.Path, "/bot"+token+"/")
	switch method {
	case "getMe":
		_, _ = io.WriteString(w, a.getMe)
	case "getUpdates":
		a.mu.Lock()
		ups, fail := a.updates, a.failPolls > 0
		a.updates = nil
		if fail {
			a.failPolls--
		}
		a.mu.Unlock()
		if fail {
			_, _ = io.WriteString(w, `{"ok":false,"error_code":502,"description":"Bad Gateway"}`)
			return
		}
		if len(ups) == 0 {
			time.Sleep(20 * time.Millisecond) // behave like a short long-poll
		}
		body, _ := json.Marshal(map[string]any{"ok": true, "result": ups})
		_, _ = w.Write(body)
	case "sendMessage":
		_ = r.ParseMultipartForm(1 << 20)
		form := map[string]string{}
		for k, v := range r.MultipartForm.Value {
			form[k] = v[0]
		}
		a.mu.Lock()
		a.sends = append(a.sends, form)
		a.mu.Unlock()
		_, _ = io.WriteString(w, a.sendResp)
		a.sent <- struct{}{}
	default:
		http.Error(w, "unexpected method "+method, http.StatusNotFound)
	}
}

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(token, slog.New(slog.DiscardHandler), bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	c.retry.sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestNewClientRejectsBadToken(t *testing.T) {
	api, srv := newFakeAPI(t)
	api.getMe = `{"ok":false,"error_code":401,"description":"Unauthorized"}`
	if _, err := NewClient(token, slog.New(slog.DiscardHandler), bot.WithServerURL(srv.URL)); !errors.Is(err, bot.ErrorUnauthorized) {
		t.Fatalf("err = %v, want unauthorized", err)
	}
}

func TestSendText(t *testing.T) {
	t.Run("reply", func(t *testing.T) {
		api, srv := newFakeAPI(t)
		id, err := newTestClient(t, srv).SendText(context.Background(), 111, "halo", 42)
		if err != nil || id != 77 {
			t.Fatalf("got %d, %v; want 77, nil", id, err)
		}
		got := api.sends[0]
		if got["chat_id"] != "111" || got["text"] != "halo" || !strings.Contains(got["reply_parameters"], `"message_id":42`) {
			t.Errorf("form = %v", got)
		}
	})

	t.Run("no reply target", func(t *testing.T) {
		api, srv := newFakeAPI(t)
		if _, err := newTestClient(t, srv).SendText(context.Background(), 111, "halo", 0); err != nil {
			t.Fatal(err)
		}
		if _, ok := api.sends[0]["reply_parameters"]; ok {
			t.Errorf("form = %v, want no reply_parameters", api.sends[0])
		}
	})

	t.Run("permanent error is not retried", func(t *testing.T) {
		api, srv := newFakeAPI(t)
		api.sendResp = `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`
		_, err := newTestClient(t, srv).SendText(context.Background(), 111, "halo", 0)
		if !errors.Is(err, bot.ErrorBadRequest) || len(api.sends) != 1 {
			t.Errorf("err = %v after %d calls; want bad request after 1", err, len(api.sends))
		}
	})

	t.Run("429 is retried", func(t *testing.T) {
		api, srv := newFakeAPI(t)
		api.sendResp = `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":1}}`
		_, err := newTestClient(t, srv).SendText(context.Background(), 111, "halo", 0)
		if err == nil || len(api.sends) != 5 {
			t.Errorf("err = %v after %d calls; want error after 5", err, len(api.sends))
		}
	})
}

// TestRunRoundTrip polls one update from the fake API, handles it and checks
// the reply goes back through sendMessage.
func TestRunRoundTrip(t *testing.T) {
	api, srv := newFakeAPI(t)
	api.updates = []models.Update{{
		ID:      9001,
		Message: &models.Message{ID: 5, Date: 1_764_300_000, Chat: models.Chat{ID: 111}, Text: "/help"},
	}}
	c := newTestClient(t, srv)
	messages, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{
		AllowedChatIDs: []int64{111},
		Log: func(context.Context, int64, string, time.Time) (expense.Expense, error) {
			return expense.Expense{}, nil
		},
		MarkProcessed:   func(context.Context, int64) (bool, error) { return true, nil },
		UnmarkProcessed: func(context.Context, int64) error { return nil },
		Notifier:        c,
		Messages:        messages,
		Logger:          slog.New(slog.DiscardHandler),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { c.Run(ctx, h); close(done) }()

	select {
	case <-api.sent:
	case <-ctx.Done():
		t.Fatal("no reply sent")
	}
	cancel()
	<-done

	got := api.sends[0]
	if got["chat_id"] != "111" || got["text"] != messages.T(i18n.Usage) {
		t.Errorf("reply form = %v", got)
	}
}

func TestDispatchSurvivesShutdown(t *testing.T) {
	_, srv := newFakeAPI(t)
	c := newTestClient(t, srv)
	messages, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	logCtxErr := errors.New("not called")
	c.handler = NewHandler(Deps{
		AllowedChatIDs: []int64{111},
		Log: func(ctx context.Context, _ int64, _ string, _ time.Time) (expense.Expense, error) {
			logCtxErr = ctx.Err()
			return expense.Expense{}, nil
		},
		MarkProcessed:   func(context.Context, int64) (bool, error) { return true, nil },
		UnmarkProcessed: func(context.Context, int64) error { return nil },
		Notifier:        c,
		Messages:        messages,
		Logger:          slog.New(slog.DiscardHandler),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // shutdown already started
	c.dispatch(ctx, nil, &models.Update{ID: 1, Message: &models.Message{ID: 5, Chat: models.Chat{ID: 111}, Text: "kopi 25k"}})

	if logCtxErr != nil {
		t.Errorf("handler ctx err = %v, want a live context", logCtxErr)
	}

	logCtxErr = errors.New("not called")
	c.dispatch(context.Background(), nil, &models.Update{ID: 2, EditedMessage: &models.Message{Chat: models.Chat{ID: 111}, Text: "x"}})
	if logCtxErr == nil {
		t.Error("edited message must not reach the handler")
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestRunLogsPollingErrors(t *testing.T) {
	api, srv := newFakeAPI(t)
	api.failPolls = 1
	logs := &syncBuffer{}
	c, err := NewClient(token, slog.New(slog.NewTextHandler(logs, nil)), bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx, NewHandler(Deps{Logger: slog.New(slog.DiscardHandler)})); close(done) }()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "telegram polling") {
		if time.Now().After(deadline) {
			t.Fatalf("polling error not logged; logs:\n%s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if strings.Contains(logs.String(), token) {
		t.Error("logs contain the bot token")
	}
}

func TestToUpdate(t *testing.T) {
	msg := &models.Message{ID: 5, Date: 1_764_300_000, Chat: models.Chat{ID: 111}, Text: "kopi 25k"}
	tests := []struct {
		name   string
		in     *models.Update
		want   Update
		wantOK bool
	}{
		{name: "nil", in: nil},
		{name: "not a new message", in: &models.Update{ID: 1, EditedMessage: msg}},
		{
			name: "message", in: &models.Update{ID: 1, Message: msg}, wantOK: true,
			want: Update{ID: 1, ChatID: 111, MessageID: 5, Text: "kopi 25k", SentAt: time.Unix(1_764_300_000, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toUpdate(tt.in)
			if ok != tt.wantOK || fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("got %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
