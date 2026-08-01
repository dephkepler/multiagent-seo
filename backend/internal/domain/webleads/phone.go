package webleads

import (
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// defaultPhoneRegion resolves phone numbers written in local format
// (no country code, e.g. "0976004626") — the site is Ukrainian, so those
// are assumed to be Ukrainian mobile numbers. Numbers that already start
// with "+" (or "00") are parsed by their own country code regardless of
// this default.
const defaultPhoneRegion = "UA"

// normalizePhone brings a phone number to the E.164 form used everywhere
// else (+380971234567, +491709007231, ...). If it can't be parsed as a
// valid number, the original text is kept as-is rather than discarded —
// a lead with an odd-looking phone number is still a lead.
func normalizePhone(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}

	num, err := phonenumbers.Parse(raw, defaultPhoneRegion)
	if err != nil || !phonenumbers.IsValidNumber(num) {
		return raw
	}
	return phonenumbers.Format(num, phonenumbers.E164)
}
