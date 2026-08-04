package linkbuilding

import "strings"

const (
	OutcomePlaced         = "placed"
	OutcomeBadCredentials = "bad_credentials"
	OutcomeCaptcha        = "captcha"
	OutcomeCloudflare     = "cloudflare"
	OutcomeLocked         = "locked"
	OutcomeTwoFactor      = "two_factor"
	OutcomeDNS            = "dns"
	OutcomeNoMarker       = "no_marker"
	OutcomeAppPasswordsOff = "app_passwords_off"
	OutcomeNoTarget       = "no_target"
	OutcomePending        = "pending"
	OutcomeLinkMissing    = "link_missing"
	OutcomeLoginFailed    = "login_failed"
	OutcomePostFailed     = "post_failed"
	OutcomeError          = "error"
)

var outcomeLabel = map[string]string{
	OutcomePlaced:         "ok",
	OutcomeBadCredentials: "bad creds",
	OutcomeCaptcha:        "captcha",
	OutcomeCloudflare:     "cloudflare",
	OutcomeLocked:         "locked",
	OutcomeTwoFactor:      "2fa",
	OutcomeDNS:            "dead domain",
	OutcomeNoMarker:        "custom login",
	OutcomeAppPasswordsOff: "app passwords off",
	OutcomeNoTarget:        "no editable target",
	OutcomeLinkMissing:     "link not visible",
	OutcomeLoginFailed:     "login failed",
	OutcomePostFailed:      "post failed",
	OutcomeError:           "error",
}

// permanentOutcomes won't change on a retry without manual action (fix creds,
// solve captcha by hand), so a re-run skips donors marked with one of these.
var permanentOutcomes = map[string]bool{
	OutcomeBadCredentials:  true,
	OutcomeCaptcha:         true,
	OutcomeTwoFactor:       true,
	OutcomeDNS:             true,
	OutcomeAppPasswordsOff: true,
	OutcomeNoTarget:        true,
}

func NormalizeDonorURL(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	return strings.TrimRight(s, "/")
}

func ShortStatus(ok bool, outcome, date string) string {
	if outcome == OutcomePending {
		return "pending review (" + date + ")"
	}
	if ok {
		return "ok (" + date + ")"
	}
	label := outcomeLabel[outcome]
	if label == "" {
		label = "failed"
	}
	return "fail: " + label + " (" + date + ")"
}

// RightsSummary is a human-readable digest of what we may do on a donor, written
// to the sheet so the operator sees the role and abilities at a glance.
func RightsSummary(caps DonorCapabilities) string {
	var can []string
	if caps.CanEditPages {
		can = append(can, "pages")
	}
	if caps.CanEditOthers {
		can = append(can, "edit-others")
	}
	if caps.CanPublish {
		can = append(can, "publish")
	}
	if caps.CanCreate {
		can = append(can, "create")
	}
	abilities := "nothing"
	if len(can) > 0 {
		abilities = strings.Join(can, "+")
	}
	role := strings.Join(caps.Roles, ",")
	if role == "" {
		return abilities
	}
	return role + ": " + abilities
}

// IsPermanentStatus reports whether a column-H cell marks a donor as not worth
// retrying. Clearing the cell by hand re-enables the donor on the next run.
func IsPermanentStatus(h string) bool {
	low := strings.ToLower(strings.TrimSpace(h))
	// A pending submission is skipped so a re-run doesn't pile up duplicate drafts.
	if strings.HasPrefix(low, "pending") {
		return true
	}
	if !strings.HasPrefix(low, "fail:") {
		return false
	}
	for code := range permanentOutcomes {
		if strings.Contains(low, outcomeLabel[code]) {
			return true
		}
	}
	return false
}

func ClassifyLoginOutcome(reason string) string {
	low := strings.ToLower(reason)
	switch {
	case strings.Contains(low, "captcha"):
		return OutcomeCaptcha
	case strings.Contains(low, "cloudflare") || strings.Contains(low, "challenge"):
		return OutcomeCloudflare
	case strings.Contains(low, "rejected the credentials") || strings.Contains(low, "incorrect") || strings.Contains(low, "invalid username"):
		return OutcomeBadCredentials
	case strings.Contains(low, "rate-limited") || strings.Contains(low, "lockout") || strings.Contains(low, "locked") || strings.Contains(low, "too many"):
		return OutcomeLocked
	case strings.Contains(low, "application_passwords_disabled") || strings.Contains(low, "application passwords"):
		return OutcomeAppPasswordsOff
	case strings.Contains(low, "two-factor") || strings.Contains(low, "2fa"):
		return OutcomeTwoFactor
	case strings.Contains(low, "no such host") || strings.Contains(low, "lookup ") || strings.Contains(low, "dial tcp"):
		return OutcomeDNS
	case strings.Contains(low, "404") || strings.Contains(low, "no known marker") || strings.Contains(low, "not wordpress"):
		return OutcomeNoMarker
	default:
		return OutcomeLoginFailed
	}
}
