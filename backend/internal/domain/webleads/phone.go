package webleads

import (
	"strings"

	"github.com/nyaruka/phonenumbers"
)

const defaultPhoneRegion = "UA"

func NormalizePhone(raw string) string {
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
