package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Canaries are assembled at runtime so no literal credential-shaped string
// sits in the source for a repo secret scanner to flag.
var (
	canarySK       = "sk-" + strings.Repeat("a1B2c3D4", 6)
	canaryGHP      = "ghp_" + strings.Repeat("a1B2c3", 6)
	canaryAKIA     = "AKIA" + "ABCDEFGHIJKLMNOP"
	canarySlack    = "xoxb-" + "123456789012-abcdefghijkl"
	canaryJWT      = "eyJhbGciOiJIUzI1NiJ9" + ".eyJzdWIiOiIxMjM0NTY3ODkwIn0" + ".c2lnbmF0dXJlLXZhbHVl"
	canaryBearer   = "Authorization: Bearer " + strings.Repeat("abcd1234", 4)
	canaryURLCreds = "postgres://dbuser:" + "s3cretpw" + "@db.internal/app"
	canaryPassword = "password=" + "hunter2hunter2"
	canaryPEM      = "-----BEGIN RSA " + "PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END RSA PRIVATE KEY-----"
)

func TestSanitizeStopTail_CanaryFamilies(t *testing.T) {
	t.Parallel()

	secretOf := map[string]string{
		"sk":       canarySK,
		"ghp":      canaryGHP,
		"akia":     canaryAKIA,
		"slack":    canarySlack,
		"jwt":      canaryJWT,
		"bearer":   strings.Repeat("abcd1234", 4),
		"url":      "s3cretpw",
		"password": "hunter2hunter2",
	}
	tails := map[string]string{
		"sk":       "calling api with " + canarySK + " now",
		"ghp":      "clone failed for token " + canaryGHP,
		"akia":     "found " + canaryAKIA + " in env",
		"slack":    "notify with " + canarySlack,
		"jwt":      "session " + canaryJWT + " expired",
		"bearer":   "retry\n" + canaryBearer + "\ndone",
		"url":      "connect to " + canaryURLCreds + " failed",
		"password": "login " + canaryPassword + " rejected",
	}

	for family, tail := range tails {
		family, tail := family, tail
		t.Run(family, func(t *testing.T) {
			t.Parallel()
			got := sanitizeStopTail(tail)
			if secret := secretOf[family]; strings.Contains(got, secret) {
				t.Errorf("canary %q survived: %q", secret, got)
			}
			if got == "" {
				t.Errorf("sanitized output is empty; a secret tail must be redacted or withheld, not dropped")
			}
		})
	}

	t.Run("pem block withholds whole tail", func(t *testing.T) {
		t.Parallel()
		got := sanitizeStopTail("before\n" + canaryPEM + "\nafter")
		if got != stopTailWithheldMarker {
			t.Errorf("got %q, want withheld marker", got)
		}
	})
}

func TestSanitizeStopTail_OneLineAndCap(t *testing.T) {
	t.Parallel()

	t.Run("newlines become separator", func(t *testing.T) {
		t.Parallel()
		got := sanitizeStopTail("first line\n\n  second   line \nthird")
		want := "first line ⏎ second line ⏎ third"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	cases := map[string]string{
		"exactly at cap": strings.Repeat(".", stopTailSinkMaxBytes),
		"cap plus one":   strings.Repeat(".", stopTailSinkMaxBytes+1),
		"multibyte":      strings.Repeat("é", stopTailSinkMaxBytes), // 2048 bytes, cut lands inside a rune for odd shifts
		"odd multibyte":  "x" + strings.Repeat("€", stopTailSinkMaxBytes),
	}
	for name, in := range cases {
		name, in := name, in
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeStopTail(in)
			if len(got) > stopTailSinkMaxBytes {
				t.Errorf("len = %d, want <= %d", len(got), stopTailSinkMaxBytes)
			}
			if !utf8.ValidString(got) {
				t.Errorf("result is not valid UTF-8")
			}
			if strings.Contains(got, "\n") {
				t.Errorf("result is not one line")
			}
		})
	}
}

func TestSanitizeStopTail_EmptyAndInvalidInput(t *testing.T) {
	t.Parallel()

	if got := sanitizeStopTail(""); got != "" {
		t.Errorf("empty input: got %q", got)
	}
	if got := sanitizeStopTail("  \n\t\n"); got != "" {
		t.Errorf("whitespace-only input: got %q", got)
	}
	if got := sanitizeStopTail("ok\xff\xfe done"); !utf8.ValidString(got) || got != "ok done" {
		t.Errorf("invalid UTF-8 input: got %q", got)
	}
}

func TestSanitizeStopTail_NonSecretTextPreserved(t *testing.T) {
	t.Parallel()

	tail := "╭──────────────────╮\n│ > Try \"fix the bug\" │\n╰──────────────────╯\n  ? for shortcuts"
	got := sanitizeStopTail(tail)
	want := "╭──────────────────╮ ⏎ │ > Try \"fix the bug\" │ ⏎ ╰──────────────────╯ ⏎ ? for shortcuts"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeStopTail_FailClosedResiduals(t *testing.T) {
	t.Parallel()

	awsLine := "AWS_SECRET_ACCESS_KEY=" + "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	clientLine := "export client_secret_key=" + "abcd1234efgh"
	keyBody := "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj"

	t.Run("env style names are not caught by redactErrorDetail alone", func(t *testing.T) {
		t.Parallel()
		for _, line := range []string{awsLine, clientLine} {
			if redactErrorDetail(line) != line {
				t.Errorf("redactErrorDetail now covers %q; the fail-closed rule may be redundant", line)
			}
		}
	})

	t.Run("env style names are withheld", func(t *testing.T) {
		t.Parallel()
		for _, line := range []string{awsLine, clientLine} {
			if got := sanitizeStopTail("ok\n" + line + "\nok"); got != stopTailWithheldMarker {
				t.Errorf("%q: got %q, want withheld", line, got)
			}
		}
	})

	t.Run("headerless base64 key body mid-tail is withheld", func(t *testing.T) {
		t.Parallel()
		got := sanitizeStopTail("line one\n" + keyBody + "\nline three")
		if got != stopTailWithheldMarker {
			t.Errorf("got %q, want withheld", got)
		}
	})

	t.Run("accepted residual: short secret in free prose passes through", func(t *testing.T) {
		t.Parallel()
		// Pinned limit, not a goal: no marker, no known shape, so nothing can catch it.
		got := sanitizeStopTail("the password is hunter2")
		if got != "the password is hunter2" {
			t.Errorf("got %q; the documented residual changed, update the plan's Deferred note", got)
		}
	})
}

func TestSanitizeStopTail_FailClosedRulesNoFalsePositiveOnCommonTails(t *testing.T) {
	t.Parallel()

	passes := map[string]string{
		"60 char url line":   "see https://github.com/tstapler/stapler-squad/blob/main/README.md",
		"50 char file path":  "/home/tstapler/code/stapler-squad/session/domain/x.go",
		"idle prompt":        "> \n  ? for shortcuts",
		"git status":         "On branch main\nChanges not staged for commit:\n\tmodified:   session/autonomous_driver.go\n",
		"max_tokens setting": "max_tokens=4096",
		"monkey assignment":  "monkey=banana",
	}
	for name, tail := range passes {
		name, tail := name, tail
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeStopTail(tail); got == stopTailWithheldMarker || got == "" {
				t.Errorf("%q was withheld/emptied: %q", tail, got)
			}
		})
	}

	t.Run("API_KEY config dump line is withheld (intended cost)", func(t *testing.T) {
		t.Parallel()
		if got := sanitizeStopTail("config:\nAPI_KEY=\nDEBUG=1"); got != stopTailWithheldMarker {
			t.Errorf("got %q, want withheld", got)
		}
	})
}
