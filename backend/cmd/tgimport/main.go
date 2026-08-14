// tgimport walks every 1:1 personal chat on the account logged in via
// cmd/tgsession and saves who's on the other end — name, @username,
// telegram id, phone if Telegram exposes it — into telegram_contacts.
// It does not fetch message history: that goes through messages.getHistory,
// which is a separate (and far stricter) Telegram flood limit than listing
// dialogs; a fresh api_id trips it hard on bulk reads (see git log for this
// file). Upsert by Telegram user id, so it's safe to rerun any time to pick
// up new contacts.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"multiagent-seo/internal/domain/correspondence"
	"multiagent-seo/internal/infrastructure/db"
	"multiagent-seo/internal/infrastructure/persistence/postgres"
	"multiagent-seo/pkg/config"
)

// pageIterator matches dialogs.Iterator (and anything with the same shape).
type pageIterator[T any] interface {
	Next(ctx context.Context) bool
	Value() T
	Err() error
}

// forEach drains it, calling visit for every element. On FLOOD_WAIT it
// sleeps the required duration and resumes the same iterator — Next()
// leaves the offset untouched on error, so the retry continues from the
// same page rather than re-fetching everything before it.
func forEach[T any](ctx context.Context, it pageIterator[T], visit func(context.Context, T) error) error {
	for {
		if !it.Next(ctx) {
			err := it.Err()
			if err == nil {
				return nil
			}
			wait, ok := tgerr.AsFloodWait(err)
			if !ok {
				return err
			}
			fmt.Printf("flood wait: sleeping %s...\n", wait)
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := visit(ctx, it.Value()); err != nil {
			return err
		}
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load:", err)
		os.Exit(1)
	}
	if cfg.TelegramUser.APIID == 0 || cfg.TelegramUser.APIHash == "" {
		fmt.Fprintln(os.Stderr, "set CF_TELEGRAM_USER_API_ID / CF_TELEGRAM_USER_API_HASH first")
		os.Exit(1)
	}

	ctx := context.Background()

	database := db.NewDatabase(cfg.Database)
	if err := database.Initialize(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "database connect:", err)
		os.Exit(1)
	}
	defer database.Close()
	store := postgres.NewCorrespondenceRepository(database.Pool())

	client := telegram.NewClient(cfg.TelegramUser.APIID, cfg.TelegramUser.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.TelegramUser.SessionFile},
	})

	err = client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		if !status.Authorized {
			return fmt.Errorf("not logged in — run `make tgsession` first")
		}

		exclude := make(map[string]bool, len(cfg.TelegramUser.Exclude))
		for _, u := range cfg.TelegramUser.Exclude {
			exclude[strings.ToLower(strings.TrimPrefix(strings.TrimSpace(u), "@"))] = true
		}

		api := client.API()
		var contactCount int

		err = forEach(ctx, query.GetDialogs(api).Iter(), func(ctx context.Context, d dialogs.Elem) error {
			// Only 1:1 personal chats — groups/channels/bots aren't
			// client correspondence and are out of scope here.
			inputUser, ok := d.Peer.(*tg.InputPeerUser)
			if !ok {
				return nil
			}
			user, ok := d.Entities.User(inputUser.UserID)
			if !ok || user.Self || user.Bot || user.Deleted {
				return nil
			}
			if exclude[strings.ToLower(user.Username)] {
				fmt.Printf("skipping %s %s (@%s) — excluded\n", user.FirstName, user.LastName, user.Username)
				return nil
			}

			if _, err := store.UpsertContact(ctx, correspondence.Contact{
				TelegramUserID: user.ID,
				Username:       user.Username,
				FirstName:      user.FirstName,
				LastName:       user.LastName,
				Phone:          user.Phone,
			}); err != nil {
				return fmt.Errorf("upsert contact %d: %w", user.ID, err)
			}
			contactCount++
			fmt.Printf("%s %s (@%s) phone=%s\n", user.FirstName, user.LastName, user.Username, user.Phone)
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk dialogs: %w", err)
		}

		fmt.Printf("Done: %d contacts.\n", contactCount)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
