// Package tgauth verifies Telegram Mini App init data.
//
// A Mini App has no login: Telegram hands the page a signed string when it
// opens, and the server checks that signature with the bot token. So this is
// the whole authentication story for a client or an advocate — no password, no
// session, no token table. Admins still sign in with a password; see
// middleware.Authenticate for how the two meet.
//
// Standard library only, on purpose: this is thirty lines of HMAC and the one
// thing that must not depend on someone else's release cadence.
package tgauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMissingHash = errors.New("tgauth: init data has no hash field")
	ErrBadHash     = errors.New("tgauth: hash mismatch")
	ErrStale       = errors.New("tgauth: auth_date is too old")
	ErrFuture      = errors.New("tgauth: auth_date is in the future")
	ErrNoUser      = errors.New("tgauth: init data has no user field")
	ErrMalformed   = errors.New("tgauth: malformed init data")
)

// futureSkew is how far ahead of us auth_date may sit before we call it
// nonsense. Clocks disagree by seconds, not minutes, but a browser with a
// badly-set clock should not be locked out over a second of drift.
const futureSkew = time.Minute

// User is the subset of Telegram's user object we actually use. ID is a
// Telegram user id — not users.id. Everything downstream needs that
// distinction, so the field name says which one it is.
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

// InitData is one verified launch.
type InitData struct {
	User     User
	AuthDate time.Time
	// StartParam carries the ?startapp= value, if the app was opened through a
	// parameterised direct link. Unused today; parsed because it is free.
	StartParam string
}

// Verifier checks init data against one bot token.
//
// Safe for concurrent use: everything on it is read-only after construction.
type Verifier struct {
	secret []byte
	maxAge time.Duration
	now    func() time.Time
}

// NewVerifier derives the signing key from the bot token once. maxAge bounds
// how long after Telegram minted it a launch stays acceptable; see the note on
// Verify about why that window is generous.
func NewVerifier(botToken string, maxAge time.Duration) (*Verifier, error) {
	if strings.TrimSpace(botToken) == "" {
		return nil, errors.New("tgauth: bot token is empty")
	}
	if maxAge <= 0 {
		return nil, errors.New("tgauth: maxAge must be positive")
	}
	return &Verifier{
		secret: secretKey(botToken),
		maxAge: maxAge,
		now:    time.Now,
	}, nil
}

// secretKey is Telegram's derivation: the HMAC of the bot token under the
// constant string "WebAppData" — note that the constant is the *key* and the
// token is the *message*, which is the reverse of what most people write first.
func secretKey(botToken string) []byte {
	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(botToken))
	return mac.Sum(nil)
}

// Verify checks the signature and freshness of raw — the exact string the page
// read from Telegram.WebApp.initData — and returns what it carries.
//
// On auth_date: Telegram stamps it once when the Mini App opens and never
// refreshes it while the app stays open, so this is the age of the launch, not
// of the request. A short window would sign out someone who left the dashboard
// on screen. The credential is therefore long-lived, which is exactly why it
// belongs in a header rather than a URL.
func (v *Verifier) Verify(raw string) (InitData, error) {
	if strings.TrimSpace(raw) == "" {
		return InitData{}, ErrMalformed
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}

	got := values.Get("hash")
	if got == "" {
		return InitData{}, ErrMissingHash
	}

	if !hmac.Equal([]byte(expectedHash(v.secret, values)), []byte(got)) {
		return InitData{}, ErrBadHash
	}

	authDate, err := parseAuthDate(values.Get("auth_date"))
	if err != nil {
		return InitData{}, err
	}
	now := v.now()
	if authDate.After(now.Add(futureSkew)) {
		return InitData{}, ErrFuture
	}
	if now.Sub(authDate) > v.maxAge {
		return InitData{}, ErrStale
	}

	rawUser := values.Get("user")
	if rawUser == "" {
		// Direct-link launches do carry a user; the field is absent for launches
		// we do not support (an inline-mode query, say). Treated as unusable
		// rather than anonymous — every endpoint is per-user data.
		return InitData{}, ErrNoUser
	}
	var user User
	if err := json.Unmarshal([]byte(rawUser), &user); err != nil {
		return InitData{}, fmt.Errorf("%w: user field: %v", ErrMalformed, err)
	}
	if user.ID == 0 {
		return InitData{}, fmt.Errorf("%w: user has no id", ErrMalformed)
	}

	return InitData{
		User:       user,
		AuthDate:   authDate,
		StartParam: values.Get("start_param"),
	}, nil
}

// expectedHash builds the data-check-string and signs it.
//
// Two details decide whether this works in production:
//
//   - The values are the *decoded* ones from url.ParseQuery, not slices of the
//     raw query string. Signing the percent-encoded form passes every test with
//     an ASCII name and fails the moment a user has a space or an emoji in
//     theirs — an intermittent, per-user 401.
//   - Only "hash" is excluded. Everything else the client sent goes into the
//     string, "signature" included. Dropping that one as well is tempting,
//     since it exists for a different purpose — the Ed25519 third-party
//     check — but it is *that* check whose string omits both fields; the
//     bot-token HMAC omits only the hash. Excluding it here rejected every
//     real launch with "hash mismatch" while a full unit-test suite stayed
//     green, because the tests encoded the same wrong assumption as the code.
func expectedHash(secret []byte, values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "hash" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(values.Get(key))
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(b.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

func parseAuthDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("%w: no auth_date", ErrMalformed)
	}
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: auth_date %q", ErrMalformed, s)
	}
	return time.Unix(secs, 0).UTC(), nil
}

// DevVerifier accepts anything and reports one fixed Telegram user.
//
// It exists because a Mini App cannot be opened in an ordinary browser: only
// Telegram can sign a launch, and Telegram will only open an https origin. So
// without this there is no way to run the client app against a local server at
// all.
//
// It is a hole in authentication and named to say so. root refuses to start
// with it configured against anything but a localhost admin URL, so it cannot
// reach a deployment.
type DevVerifier struct {
	userID int64
}

func NewDevVerifier(userID int64) *DevVerifier {
	return &DevVerifier{userID: userID}
}

func (v *DevVerifier) Verify(string) (InitData, error) {
	return InitData{
		User:     User{ID: v.userID, FirstName: "Dev"},
		AuthDate: time.Now(),
	}, nil
}
