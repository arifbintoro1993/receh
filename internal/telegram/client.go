package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Client receives updates by long polling and implements Notifier.
type Client struct {
	b       *bot.Bot
	retry   retrier
	handler *Handler
}

var _ Notifier = (*Client)(nil)

// NewClient checks the token with getMe, so a bad token fails at startup.
// extra options are applied last, e.g. bot.WithServerURL for a self-hosted
// Bot API server or a test server.
func NewClient(token string, logger *slog.Logger, extra ...bot.Option) (*Client, error) {
	c := &Client{retry: newRetrier()}
	opts := append([]bot.Option{
		bot.WithDefaultHandler(c.dispatch),
		bot.WithAllowedUpdates(bot.AllowedUpdates{"message"}),
		bot.WithErrorsHandler(func(err error) { logger.Warn("telegram polling", "err", err) }),
	}, extra...)
	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	c.b = b
	return c, nil
}

// Run polls for updates and hands them to h until ctx is cancelled.
func (c *Client) Run(ctx context.Context, h *Handler) {
	c.handler = h
	c.b.Start(ctx)
}

// handleTimeout bounds one update, including during shutdown.
const handleTimeout = 15 * time.Second

func (c *Client) dispatch(ctx context.Context, _ *bot.Bot, u *models.Update) {
	upd, ok := toUpdate(u)
	if !ok {
		return
	}
	// The library cancels ctx on shutdown while handlers still run. Detach so
	// an update already marked processed finishes saving instead of failing
	// half-way and being skipped as a duplicate on redelivery.
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), handleTimeout)
	defer cancel()
	c.handler.Handle(hctx, upd)
}

func (c *Client) SendText(ctx context.Context, chatID int64, text string, replyTo int) (int, error) {
	params := &bot.SendMessageParams{ChatID: chatID, Text: text}
	if replyTo != 0 {
		params.ReplyParameters = &models.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true}
	}
	var sent *models.Message
	err := c.retry.do(ctx, func() error {
		var err error
		sent, err = c.b.SendMessage(ctx, params)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("send message: %w", err)
	}
	return sent.ID, nil
}

// toUpdate keeps only new messages; edits, channel posts and others are ignored.
func toUpdate(u *models.Update) (Update, bool) {
	if u == nil || u.Message == nil {
		return Update{}, false
	}
	m := u.Message
	return Update{
		ID:        u.ID,
		ChatID:    m.Chat.ID,
		MessageID: m.ID,
		Text:      m.Text,
		SentAt:    time.Unix(int64(m.Date), 0),
	}, true
}
