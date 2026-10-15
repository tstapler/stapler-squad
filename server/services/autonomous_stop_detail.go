package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	// stopTailWithheldMarker replaces a whole tail that matched a secret rule.
	stopTailWithheldMarker = "[tail withheld: secret pattern detected]"
	// stopTailSinkMaxBytes caps the sanitized tail stored in the stuck-row context.
	stopTailSinkMaxBytes = 1024
	// stopTailLineSeparator joins the tail's lines into one line.
	stopTailLineSeparator = " ⏎ "
	// keyBodyMinLen is the shortest line treated as headerless key material.
	keyBodyMinLen = 40
)

// secretNameSegments are the NAME segments that mark an assignment as secret-bearing.
var secretNameSegments = map[string]struct{}{
	"SECRET": {}, "TOKEN": {}, "PASSWORD": {}, "PASSWD": {},
	"KEY": {}, "CREDENTIAL": {}, "CREDENTIALS": {},
}

// sanitizeStopTail makes a bounded pane tail safe for a durable sink. It
// withholds the whole tail when any secret rule fires (fail closed), else
// redacts known credential shapes, folds the tail to one line and caps it at
// stopTailSinkMaxBytes, keeping the end. Accepted residual: a short secret in
// free prose with no marker, or one split across the capture's line cut.
func sanitizeStopTail(raw string) string {
	// Invisible runes are dropped first so they cannot split a token past the rules.
	s := dropInvisibleRunes(strings.ToValidUTF8(raw, ""))
	if strings.TrimSpace(s) == "" {
		return ""
	}
	if session.RunPreGateSecurityCheck(s) != nil || tailHasSecretLine(s) || vendorTokenRe.MatchString(s) {
		return stopTailWithheldMarker
	}

	s = redactErrorDetail(s)
	return session.TailBytes(foldToOneLine(s), stopTailSinkMaxBytes)
}

// vendorTokenRe withholds the tail on vendor token prefixes and `curl -u user:pass`.
var vendorTokenRe = regexp.MustCompile(`\b(?:glpat-[A-Za-z0-9_-]{20,}|AIza[0-9A-Za-z_-]{35}|pypi-[A-Za-z0-9_-]{20,}|hf_[A-Za-z0-9]{30,}|dop_v1_[a-f0-9]{20,}|rk_live_[A-Za-z0-9]{16,}|sk-ant-[A-Za-z0-9_-]{20,})|\bcurl\b[^\n]*\s(?:-u|--user)[\s=]+\S*:\S+`)

// dropInvisibleRunes removes control (except \n, \r, \t) and bidi/format runes.
func dropInvisibleRunes(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return r
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
}

func tailHasSecretLine(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if hasSecretEnvAssignment(line) || looksLikeKeyBody(line) {
			return true
		}
	}
	return false
}

// hasSecretEnvAssignment reports whether line contains NAME=value where NAME is
// made only of letters, digits, '_' and '-' and has a secret-bearing segment.
// Names are matched case-insensitively: lower-case client_secret_key is covered.
func hasSecretEnvAssignment(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] != '=' {
			continue
		}
		start := i
		for start > 0 && isEnvNameByte(line[start-1]) {
			start--
		}
		name := strings.TrimLeft(line[start:i], "-")
		if name == "" {
			continue
		}
		for _, seg := range strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool { return r == '_' || r == '-' }) {
			if _, ok := secretNameSegments[seg]; ok {
				return true
			}
		}
	}
	return false
}

func isEnvNameByte(b byte) bool {
	return b == '_' || b == '-' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// looksLikeKeyBody reports whether the whole (trimmed) line is base64-alphabet
// text of at least keyBodyMinLen bytes with no "//", i.e. headerless key material
// rather than a path or URL (those contain '.', ':' or "//"). Rule lines
// ("=====", "+++++"), single-letter runs and all-lowercase-hex digests (git
// SHAs) are not key material: a key body must mix a letter and a digit.
func looksLikeKeyBody(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) < keyBodyMinLen || strings.Contains(line, "//") {
		return false
	}
	hasLetter, hasDigit, allLowerHex, allSame := false, false, true, true
	for i := 0; i < len(line); i++ {
		c := line[i]
		isDigit := c >= '0' && c <= '9'
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !isDigit && !isLetter && c != '+' && c != '/' && c != '=' {
			return false
		}
		hasDigit = hasDigit || isDigit
		hasLetter = hasLetter || isLetter
		allLowerHex = allLowerHex && (isDigit || (c >= 'a' && c <= 'f'))
		allSame = allSame && c == line[0]
	}
	return hasLetter && hasDigit && !allSame && !allLowerHex
}

// foldToOneLine collapses whitespace runs and joins non-empty lines with
// stopTailLineSeparator.
func foldToOneLine(s string) string {
	var parts []string
	for _, line := range strings.Split(s, "\n") {
		if f := strings.FieldsFunc(line, unicode.IsSpace); len(f) > 0 {
			parts = append(parts, strings.Join(f, " "))
		}
	}
	return strings.Join(parts, stopTailLineSeparator)
}

// stopAction is the final recovery recorded on the stuck row and the log line.
type stopAction string

const (
	stopActionNone         stopAction = "none"
	stopActionNudging      stopAction = "nudging" // plan-time row annotation, replaced by the final action
	stopActionNudged       stopAction = "nudged"
	stopActionRespawned    stopAction = "respawned"
	stopActionResupervised stopAction = "resupervised"
)

// nudgeOutcome classifies a nudge attempt. The code is for logs and the row's
// key=value part; only nudgeFailurePhrase text reaches an operator sentence.
type nudgeOutcome string

const (
	nudgeOutcomeNA              nudgeOutcome = "n/a"
	nudgeOutcomeSuccess         nudgeOutcome = "success"
	nudgeOutcomeDisabled        nudgeOutcome = "disabled"
	nudgeOutcomeNotIdle         nudgeOutcome = "not_idle"
	nudgeOutcomeLeaseBusy       nudgeOutcome = "lease_busy"
	nudgeOutcomeOwnership       nudgeOutcome = "ownership"
	nudgeOutcomeSlotUnavailable nudgeOutcome = "slot_unavailable"
	nudgeOutcomeUncertain       nudgeOutcome = "uncertain"
	nudgeOutcomeCtxCancelled    nudgeOutcome = "ctx_cancelled"
	nudgeOutcomeRestartFailed   nudgeOutcome = "restart_failed"
	// nudgeOutcomeNotRestartable: the session was archived, paused, retrying or had
	// autonomy turned off, so nothing was written and nothing is restarted.
	nudgeOutcomeNotRestartable nudgeOutcome = "not_restartable"
)

// allNudgeOutcomes drives the copy completeness test.
var allNudgeOutcomes = []nudgeOutcome{
	nudgeOutcomeNA, nudgeOutcomeSuccess, nudgeOutcomeDisabled, nudgeOutcomeNotIdle,
	nudgeOutcomeLeaseBusy, nudgeOutcomeOwnership, nudgeOutcomeSlotUnavailable,
	nudgeOutcomeUncertain, nudgeOutcomeCtxCancelled, nudgeOutcomeRestartFailed, nudgeOutcomeNotRestartable,
}

// noneCause says why a stop ended with no recovery, for the action sentence.
type noneCause string

const (
	// noneCauseHandled is today's handling: nothing automatic, the operator acts.
	noneCauseHandled      noneCause = ""
	noneCauseArchived     noneCause = "archived"
	noneCauseRetryPending noneCause = "retry_pending"
)

// stopResuperviseCap is the re-supervise budget shown as resupervise=n/cap until
// the re-supervise story supplies the live value.
const stopResuperviseCap = 3

// stopRecovery is what the callback finally did about a stop.
type stopRecovery struct {
	Action            stopAction
	NudgeOutcome      nudgeOutcome // "" reads as n/a
	NoneBecause       noneCause
	NudgeUsed         int
	NudgeCap          int // 0 means no nudge was in play (the nudge= key is omitted)
	ResuperviseN      int
	ResuperviseCapHit bool // the re-supervise cap is what ended the chain
}

// Reason sentences, one per stalling StopReason; done and cancelled have none.
const (
	reasonSentenceMaxTurns           = "The autonomous run used up its turn allowance before finishing."
	reasonSentenceStartupTimeout     = "The autonomous run gave up waiting for the session to become ready."
	reasonSentenceOrchestratorError  = "The autonomous run hit an internal error and stopped."
	reasonSentenceSubmitFailed       = "The autonomous run could not type its next instruction into the session."
	reasonSentenceLeaseTimeout       = "The autonomous run could not get access to the session because something else was using it."
	reasonSentenceRateLimitExhausted = "The autonomous run gave up waiting for the AI service's rate limit to clear."
	reasonSentencePanic              = "The autonomous run crashed unexpectedly."
	reasonSentenceUnspecified        = "The autonomous run stopped without recording a reason."
)

// reasonSentence returns the plain sentence for a stalling reason, "" for done
// and the cancelled reasons (they never reach an operator notification).
func reasonSentence(r domain.StopReason) string {
	switch r {
	case domain.StopReasonMaxTurns:
		return reasonSentenceMaxTurns
	case domain.StopReasonStartupTimeout:
		return reasonSentenceStartupTimeout
	case domain.StopReasonOrchestratorError:
		return reasonSentenceOrchestratorError
	case domain.StopReasonSubmitFailed:
		return reasonSentenceSubmitFailed
	case domain.StopReasonLeaseTimeout:
		return reasonSentenceLeaseTimeout
	case domain.StopReasonRateLimitExhausted:
		return reasonSentenceRateLimitExhausted
	case domain.StopReasonPanic:
		return reasonSentencePanic
	case "":
		return reasonSentenceUnspecified
	default:
		return ""
	}
}

// stateSentence returns the plain sentence for what the pane was doing.
func stateSentence(s domain.ObservedState) string {
	switch s {
	case domain.ObservedIdlePrompt:
		return "The session was waiting at its prompt."
	case domain.ObservedNeedsApproval:
		return "The session was waiting for an approval."
	case domain.ObservedIdle:
		return "The session was sitting at a shell prompt."
	case domain.ObservedWorking:
		return "The session was still working."
	case domain.ObservedError:
		return "The session was showing an error."
	case domain.ObservedExited:
		return "The session had exited."
	case domain.ObservedCrashed:
		return "The session had crashed."
	default:
		return "The session's state could not be read."
	}
}

// nudgeFailurePhrase is the operator wording for a failed nudge; "" for the
// classes that never appear in a sentence.
func nudgeFailurePhrase(o nudgeOutcome) string {
	switch o {
	case nudgeOutcomeNotIdle:
		return "the session was busy again"
	case nudgeOutcomeLeaseBusy:
		return "another process was using the session"
	case nudgeOutcomeOwnership:
		return "the session's terminal was not the one this run expected"
	case nudgeOutcomeSlotUnavailable:
		return "no automatic prompt was available for this item (limit reached or one was sent recently)"
	default:
		return ""
	}
}

const (
	actionTail         = "You only need to act if a \"needs manual attention\" notification follows."
	actionSentenceNone = "Open the session to review what was accomplished and give the next instruction."
)

// actionSentence composes the final-action sentence; "" for a re-supervise,
// which publishes nothing.
func actionSentence(r stopRecovery) string {
	switch r.Action {
	case stopActionNudging:
		return "A prompt is being sent to continue automatically."
	case stopActionNudged:
		return fmt.Sprintf("It was prompted to continue automatically (nudge %d of %d). You do not need to do anything.", r.NudgeUsed, r.NudgeCap)
	case stopActionRespawned:
		switch {
		case r.NudgeOutcome == nudgeOutcomeRestartFailed:
			return "A prompt was sent, but the autonomous run could not be re-attached, so a fresh session is being started instead. " + actionTail
		case r.ResuperviseCapHit:
			return fmt.Sprintf("The autonomous run was re-attached %d times while the session kept working, so a fresh session is being started. %s", r.ResuperviseN, actionTail)
		case nudgeFailurePhrase(r.NudgeOutcome) != "":
			return fmt.Sprintf("A prompt could not be sent (%s), so a fresh session is being started instead. %s", nudgeFailurePhrase(r.NudgeOutcome), actionTail)
		}
		return "A fresh session is being started automatically. " + actionTail
	case stopActionNone:
		switch {
		case r.NudgeOutcome == nudgeOutcomeUncertain:
			return "A prompt may or may not have reached the session, so nothing was restarted. Open the session and check it."
		case r.NoneBecause == noneCauseArchived:
			return "Nothing was restarted because the session is archived. You do not need to do anything."
		case r.NoneBecause == noneCauseRetryPending:
			return "Nothing was restarted because an automatic retry is already scheduled. You do not need to do anything."
		}
		return actionSentenceNone
	default:
		return ""
	}
}

// plainSummary is the one operator-facing sentence group shared by the
// notification body and the stuck row's lead, so the two cannot drift.
func plainSummary(reason domain.StopReason, state domain.ObservedState, r stopRecovery) string {
	var parts []string
	for _, p := range []string{reasonSentence(reason), stateSentence(state), actionSentence(r)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// stopTitle is the notification title for the final action.
func stopTitle(r stopRecovery) string {
	if r.Action == stopActionNudged {
		return "Autonomous session nudged"
	}
	return "Autonomous fix stuck"
}

// stopReasonCode is the greppable wire value; the empty legacy reason is named.
func stopReasonCode(r domain.StopReason) string {
	if r == "" {
		return "unspecified"
	}
	return string(r)
}

// stopRecord is everything one stop's row, log line and notification are built from.
type stopRecord struct {
	Session  string
	ItemID   string
	Role     string
	Outcome  session.AutonomousDriverOutcome
	State    domain.ObservedState
	Recovery stopRecovery
	MaxTurns int
	// Tail is the already-sanitized pane tail; the raw tail never leaves this file's callers.
	Tail string
}

func (r stopRecord) reason() domain.StopReason { return r.Outcome.NormalizedStopReason() }

func (r stopRecord) nudgeOutcome() nudgeOutcome {
	if r.Recovery.NudgeOutcome == "" {
		return nudgeOutcomeNA
	}
	return r.Recovery.NudgeOutcome
}

func (r stopRecord) resupervise() string {
	return fmt.Sprintf("%d/%d", r.Recovery.ResuperviseN, stopResuperviseCap)
}

// formatStopDetail builds the stuck row's context: the plain summary, then a
// one-line key=value part for greppability. Single-line by construction.
func formatStopDetail(r stopRecord) string {
	last := "unknown"
	if t := r.Outcome.Snapshot.LastActivityAt; !t.IsZero() {
		last = t.UTC().Format(time.RFC3339)
	}
	var b strings.Builder
	b.WriteString(plainSummary(r.reason(), r.State, r.Recovery))
	fmt.Fprintf(&b, " | stop_reason=%s state=%s action=%s", stopReasonCode(r.reason()), r.State, r.Recovery.Action)
	if r.Recovery.NudgeCap > 0 {
		fmt.Fprintf(&b, " nudge=%d/%d", r.Recovery.NudgeUsed, r.Recovery.NudgeCap)
	}
	fmt.Fprintf(&b, " turns=%d resupervise=%s last_activity=%s nudge_outcome=%s tail=%s",
		r.Outcome.Turns, r.resupervise(), last, r.nudgeOutcome(), r.Tail)
	return b.String()
}

// logStop emits the single "[AutonomousDriver] stopped" line for a stop. It
// never includes the tail, only its size.
func logStop(r stopRecord) {
	age := "unknown"
	snap := r.Outcome.Snapshot
	if !snap.LastActivityAt.IsZero() && !snap.CapturedAt.IsZero() {
		age = snap.CapturedAt.Sub(snap.LastActivityAt).Round(time.Second).String()
	}
	attrs := []any{
		"session", r.Session, "item", r.ItemID, "role", r.Role,
		"run_id", r.Outcome.RunID,
		"stop_reason", stopReasonCode(r.reason()), "state", string(r.State),
		"turns", r.Outcome.Turns, "max_turns", r.MaxTurns,
		"last_activity_age", age, "action", string(r.Recovery.Action),
		"tail_bytes", len(r.Tail), "resupervise", r.resupervise(),
		"nudge_outcome", string(r.nudgeOutcome()),
		"resupervise_cap_hit", r.Recovery.ResuperviseCapHit,
	}
	if r.reason().IsStall() {
		log.Warn("[AutonomousDriver] stopped", attrs...)
		return
	}
	log.Info("[AutonomousDriver] stopped", attrs...)
}
