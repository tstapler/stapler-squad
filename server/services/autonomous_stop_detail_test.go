package services

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
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

	t.Run("rule and digest lines are not key material", func(t *testing.T) {
		t.Parallel()
		sha := strings.Repeat("0123456789abcdef", 4)[:40]
		for name, line := range map[string]string{
			"equals rule":  strings.Repeat("=", 60),
			"plus rule":    strings.Repeat("+", 45),
			"letter run":   strings.Repeat("a", 50),
			"git sha":      sha,
			"sha256 hex":   strings.Repeat("0123456789abcdef", 4),
			"padded equal": "== " + strings.Repeat("=", 50),
		} {
			want := strings.TrimSpace(line)
			if got := sanitizeStopTail("ok\n" + line + "\nok"); got == stopTailWithheldMarker {
				t.Errorf("%s: %q was withheld", name, want)
			}
		}
	})

	t.Run("mixed-case base64 key body with digits is withheld", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("aB3dE5gH", 6)
		if got := sanitizeStopTail(body); got != stopTailWithheldMarker {
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

// stopCopyGolden pins the operator-facing copy byte for byte; editing a sentence
// is deliberate and must change this table in the same commit (UX G4, G5).
var stopCopyGolden = struct {
	reasons  map[domain.StopReason]string
	states   map[domain.ObservedState]string
	phrases  map[nudgeOutcome]string
	nudged   string
	respawn  string
	failed   string
	restart  string
	uncertn  string
	archived string
	retry    string
	capHit   string
	manual   string
}{
	reasons: map[domain.StopReason]string{
		domain.StopReasonMaxTurns:           "The autonomous run used up its turn allowance before finishing.",
		domain.StopReasonStartupTimeout:     "The autonomous run gave up waiting for the session to become ready.",
		domain.StopReasonOrchestratorError:  "The autonomous run hit an internal error and stopped.",
		domain.StopReasonSubmitFailed:       "The autonomous run could not type its next instruction into the session.",
		domain.StopReasonLeaseTimeout:       "The autonomous run could not get access to the session because something else was using it.",
		domain.StopReasonRateLimitExhausted: "The autonomous run gave up waiting for the AI service's rate limit to clear.",
		domain.StopReasonPanic:              "The autonomous run crashed unexpectedly.",
		"":                                  "The autonomous run stopped without recording a reason.",
		domain.StopReasonDone:               "",
		domain.StopReasonCancelledExternal:  "",
		domain.StopReasonCancelledShutdown:  "",
	},
	states: map[domain.ObservedState]string{
		domain.ObservedIdlePrompt:    "The session was waiting at its prompt.",
		domain.ObservedNeedsApproval: "The session was waiting for an approval.",
		domain.ObservedIdle:          "The session was sitting at a shell prompt.",
		domain.ObservedWorking:       "The session was still working.",
		domain.ObservedError:         "The session was showing an error.",
		domain.ObservedExited:        "The session had exited.",
		domain.ObservedCrashed:       "The session had crashed.",
		domain.ObservedUnknown:       "The session's state could not be read.",
	},
	phrases: map[nudgeOutcome]string{
		nudgeOutcomeNotIdle:         "the session was busy again",
		nudgeOutcomeLeaseBusy:       "another process was using the session",
		nudgeOutcomeOwnership:       "the session's terminal was not the one this run expected",
		nudgeOutcomeSlotUnavailable: "no automatic prompt was available for this item (limit reached or one was sent recently)",
		nudgeOutcomeNA:              "", nudgeOutcomeSuccess: "", nudgeOutcomeDisabled: "",
		nudgeOutcomeUncertain: "", nudgeOutcomeCtxCancelled: "", nudgeOutcomeRestartFailed: "", nudgeOutcomeNotRestartable: "",
	},
	nudged:   "It was prompted to continue automatically (nudge 1 of 2). You do not need to do anything.",
	respawn:  "A fresh session is being started automatically. You only need to act if a \"needs manual attention\" notification follows.",
	failed:   "A prompt could not be sent (the session was busy again), so a fresh session is being started instead. You only need to act if a \"needs manual attention\" notification follows.",
	restart:  "A prompt was sent, but the autonomous run could not be re-attached, so a fresh session is being started instead. You only need to act if a \"needs manual attention\" notification follows.",
	uncertn:  "A prompt may or may not have reached the session, so nothing was restarted. Open the session and check it.",
	archived: "Nothing was restarted because the session is archived. You do not need to do anything.",
	retry:    "Nothing was restarted because an automatic retry is already scheduled. You do not need to do anything.",
	capHit:   "The autonomous run was re-attached 3 times while the session kept working, so a fresh session is being started. You only need to act if a \"needs manual attention\" notification follows.",
	manual:   "Open the session to review what was accomplished and give the next instruction.",
}

// OBS-4: every enum value has a copy row.
func TestStopReasonPhrase_EveryReasonHasRow(t *testing.T) {
	t.Parallel()
	for _, r := range append([]domain.StopReason{""}, domain.AllStopReasons...) {
		got := reasonSentence(r)
		if r.IsStall() {
			assert.NotEmpty(t, got, "stalling reason %q needs a sentence", r)
		} else {
			assert.Empty(t, got, "%q never reaches an operator notification", r)
		}
	}
	for _, s := range domain.AllObservedStates {
		assert.NotEmpty(t, stateSentence(s), s)
	}
	for _, o := range allNudgeOutcomes {
		_, ok := stopCopyGolden.phrases[o]
		assert.True(t, ok, "nudge outcome %q is missing from the golden table", o)
	}
}

// OBS-10: golden strings, and no wire code leaks into a sentence.
func TestStopCopy_GoldenStringsAndNoCodes(t *testing.T) {
	t.Parallel()
	for r, want := range stopCopyGolden.reasons {
		assert.Equal(t, want, reasonSentence(r), "reason %q", r)
	}
	for s, want := range stopCopyGolden.states {
		assert.Equal(t, want, stateSentence(s), "state %q", s)
	}
	for o, want := range stopCopyGolden.phrases {
		assert.Equal(t, want, nudgeFailurePhrase(o), "nudge outcome %q", o)
	}
	actions := map[string]stopRecovery{
		stopCopyGolden.nudged:   {Action: stopActionNudged, NudgeUsed: 1, NudgeCap: 2},
		stopCopyGolden.respawn:  {Action: stopActionRespawned},
		stopCopyGolden.failed:   {Action: stopActionRespawned, NudgeOutcome: nudgeOutcomeNotIdle},
		stopCopyGolden.restart:  {Action: stopActionRespawned, NudgeOutcome: nudgeOutcomeRestartFailed},
		stopCopyGolden.uncertn:  {Action: stopActionNone, NudgeOutcome: nudgeOutcomeUncertain},
		stopCopyGolden.archived: {Action: stopActionNone, NoneBecause: noneCauseArchived},
		stopCopyGolden.retry:    {Action: stopActionNone, NoneBecause: noneCauseRetryPending},
		stopCopyGolden.capHit:   {Action: stopActionRespawned, ResuperviseCapHit: true, ResuperviseN: 3},
		stopCopyGolden.manual:   {Action: stopActionNone},
	}
	for want, rec := range actions {
		assert.Equal(t, want, actionSentence(rec))
	}
	assert.Empty(t, actionSentence(stopRecovery{Action: stopActionResupervised}))

	codes := make([]string, 0, len(domain.AllStopReasons)+len(domain.AllObservedStates)+len(allNudgeOutcomes))
	for _, r := range domain.AllStopReasons {
		codes = append(codes, string(r))
	}
	for _, s := range domain.AllObservedStates {
		codes = append(codes, string(s))
	}
	for _, o := range allNudgeOutcomes {
		codes = append(codes, string(o))
	}
	for _, reason := range append([]domain.StopReason{""}, domain.AllStopReasons...) {
		for _, state := range domain.AllObservedStates {
			for _, rec := range actions {
				text := plainSummary(reason, state, rec)
				for _, code := range codes {
					// Single-word codes ("idle", "working") are ordinary English; only
					// the underscore codes are unambiguous.
					if strings.Contains(code, "_") {
						assert.NotContains(t, text, code)
					}
				}
			}
		}
	}
	assert.Equal(t, "Autonomous session nudged", stopTitle(stopRecovery{Action: stopActionNudged}))
	assert.Equal(t, "Autonomous fix stuck", stopTitle(stopRecovery{Action: stopActionRespawned}))
}

func detailFor(snap domain.StopSnapshot, turns int, reason domain.StopReason) string {
	return formatStopDetail(stopRecord{
		Outcome:  session.AutonomousDriverOutcome{StopReason: reason, Turns: turns, Snapshot: snap},
		State:    domain.ClassifyObserved(snap),
		Recovery: stopRecovery{Action: stopActionRespawned},
		Tail:     sanitizeStopTail(snap.PaneTail),
	})
}

// OBS-3: idle, error, exited and crashed are told apart in the detail.
func TestStopDetail_StateDistinguishesIdleErrorExited(t *testing.T) {
	t.Parallel()
	cases := map[string]domain.StopSnapshot{
		"state=idle_prompt": {TmuxAlive: true, ControllerActive: true, Bucket: domain.BucketSafeIdle},
		"state=error":       {TmuxAlive: true, ControllerActive: true, Bucket: domain.BucketError},
		"state=exited":      {PaneDead: true, ExitCode: 0},
		"state=crashed":     {PaneDead: true, ExitCode: 1},
	}
	for want, snap := range cases {
		assert.Contains(t, detailFor(snap, 1, domain.StopReasonOrchestratorError), want)
	}
}

// OBS-8: one line, newlines shown with the marker.
func TestStopDetail_SingleLineWithMarker(t *testing.T) {
	t.Parallel()
	snap := domain.StopSnapshot{TmuxAlive: true, ControllerActive: true, Bucket: domain.BucketSafeIdle, PaneTail: "first line\nsecond line\n\nthird"}
	got := detailFor(snap, 2, domain.StopReasonOrchestratorError)
	assert.NotContains(t, got, "\n")
	assert.Contains(t, got, "tail=first line ⏎ second line ⏎ third")
}

// OBS-9 (key part): every detail carries resupervise=n/3.
func TestStopDetail_ResuperviseKey(t *testing.T) {
	t.Parallel()
	assert.Contains(t, detailFor(domain.StopSnapshot{}, 0, domain.StopReasonMaxTurns), " resupervise=0/3 ")
}
