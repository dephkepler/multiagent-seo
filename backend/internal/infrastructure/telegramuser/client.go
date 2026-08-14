// Package telegramuser wraps a long-running MTProto connection on the
// personal Telegram account (see cmd/tgsession) so the rest of the app can
// issue ad-hoc calls it — right now just creating a group chat, which the
// Bot API has no method for at all; only a user account can do it.
package telegramuser

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

type Client struct {
	api *tg.Client
	log *slog.Logger
}

// New connects and blocks until the connection is authorized (or ctx is
// cancelled / connectTimeout elapses), then returns. The connection itself
// is kept alive for the lifetime of ctx by a background goroutine — cancel
// ctx to close it.
func New(ctx context.Context, apiID int, apiHash, sessionFile string, log *slog.Logger) (*Client, error) {
	if log == nil {
		log = slog.Default()
	}
	raw := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionFile},
	})

	ready := make(chan error, 1)
	go func() {
		err := raw.Run(ctx, func(runCtx context.Context) error {
			status, err := raw.Auth().Status(runCtx)
			if err != nil {
				ready <- fmt.Errorf("auth status: %w", err)
				return err
			}
			if !status.Authorized {
				err := fmt.Errorf("not logged in — run `make tgsession` first")
				ready <- err
				return err
			}
			ready <- nil
			<-runCtx.Done() // hold the connection open for callers of api
			return runCtx.Err()
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.ErrorContext(ctx, "telegramuser: connection ended", "err", err)
		}
	}()

	select {
	case err := <-ready:
		if err != nil {
			return nil, err
		}
	case <-time.After(20 * time.Second):
		return nil, fmt.Errorf("telegramuser: connect timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &Client{api: raw.API(), log: log}, nil
}

// CreateGroup resolves each of usernames (with or without a leading @) to a
// Telegram user and creates a new basic group chat with all of them. Needs
// at least one invitee besides the account itself — Telegram rejects a
// single-member "group".
//
// Resolution only works for accounts with a public @username; a client who
// only has a phone number and no username can't be invited this way (see
// consultations.Client.TelegramName — it's empty/name-only for those).
func (c *Client) CreateGroup(ctx context.Context, title string, usernames []string) (int64, error) {
	if len(usernames) == 0 {
		return 0, fmt.Errorf("createGroup: need at least one invitee")
	}

	users := make([]tg.InputUserClass, 0, len(usernames))
	for _, raw := range usernames {
		username := strings.TrimPrefix(strings.TrimSpace(raw), "@")
		if username == "" {
			return 0, fmt.Errorf("createGroup: empty username")
		}
		resolved, err := c.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		if err != nil {
			return 0, fmt.Errorf("createGroup: resolve @%s: %w", username, err)
		}
		var input *tg.InputUser
		for _, u := range resolved.Users {
			if user, ok := u.(*tg.User); ok {
				input = user.AsInput()
				break
			}
		}
		if input == nil {
			return 0, fmt.Errorf("createGroup: @%s did not resolve to a user", username)
		}
		users = append(users, input)
	}

	res, err := c.api.MessagesCreateChat(ctx, &tg.MessagesCreateChatRequest{
		Users: users,
		Title: title,
	})
	if err != nil {
		return 0, fmt.Errorf("createGroup: %w", err)
	}

	updates, ok := res.Updates.(*tg.Updates)
	if !ok {
		return 0, fmt.Errorf("createGroup: unexpected update type %T", res.Updates)
	}
	for _, ch := range updates.Chats {
		if chat, ok := ch.(*tg.Chat); ok {
			c.log.InfoContext(ctx, "telegramuser: group created", "chat_id", chat.ID, "title", title)
			return chat.ID, nil
		}
	}
	return 0, fmt.Errorf("createGroup: no chat in response")
}
