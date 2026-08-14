// tgsession logs a personal Telegram account into MTProto and saves the
// resulting session to a file. Run it once, by hand — every later tool that
// needs to act as this account (history importer, live listener) reuses the
// saved session instead of logging in again.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"

	"multiagent-seo/pkg/config"
)

// terminalAuth answers Telegram's login flow (phone, SMS/app code, 2FA
// password) by prompting on stdin/stdout — the code and password are typed
// by whoever runs this tool, never passed on the command line or read from
// config, so they don't end up in shell history or .env.
type terminalAuth struct {
	phone  string
	reader *bufio.Reader
}

func (a *terminalAuth) Phone(context.Context) (string, error) {
	if a.phone != "" {
		return a.phone, nil
	}
	return a.prompt("Phone number, with country code (e.g. +380...): ")
}

func (a *terminalAuth) Code(context.Context, *tg.AuthSentCode) (string, error) {
	return a.prompt("Code Telegram just sent you: ")
}

func (a *terminalAuth) Password(context.Context) (string, error) {
	fmt.Print("2FA password (leave empty if you don't have one): ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(pw), nil
}

func (a *terminalAuth) AcceptTermsOfService(context.Context, tg.HelpTermsOfService) error {
	return nil
}

func (a *terminalAuth) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("this phone number has no Telegram account yet — sign up in the Telegram app first, then rerun")
}

func (a *terminalAuth) prompt(label string) (string, error) {
	fmt.Print(label)
	line, err := a.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config load:", err)
		os.Exit(1)
	}
	if cfg.TelegramUser.APIID == 0 || cfg.TelegramUser.APIHash == "" {
		fmt.Fprintln(os.Stderr, "set CF_TELEGRAM_USER_API_ID and CF_TELEGRAM_USER_API_HASH first — get them at https://my.telegram.org (API development tools), they're separate from the bot token")
		os.Exit(1)
	}

	client := telegram.NewClient(cfg.TelegramUser.APIID, cfg.TelegramUser.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.TelegramUser.SessionFile},
	})
	authenticator := &terminalAuth{phone: cfg.TelegramUser.Phone, reader: bufio.NewReader(os.Stdin)}

	err = client.Run(context.Background(), func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		if status.Authorized {
			fmt.Printf("Already logged in as %s (%s) — session at %s is still good.\n",
				status.User.Username, status.User.FirstName, cfg.TelegramUser.SessionFile)
			return nil
		}

		if err := client.Auth().IfNecessary(ctx, auth.NewFlow(authenticator, auth.SendCodeOptions{})); err != nil {
			return fmt.Errorf("login: %w", err)
		}

		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("fetch self: %w", err)
		}
		fmt.Printf("Logged in as %s (%s). Session saved to %s\n", self.Username, self.FirstName, cfg.TelegramUser.SessionFile)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
