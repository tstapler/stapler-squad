package session

import "github.com/tstapler/stapler-squad/session/detection"

// SafeIdleStatusContexts allowlists the exact detection.StatusIdle pattern
// descriptions (session/detection/binaries/claude.go's Idle group) that are
// unambiguously Claude Code's own idle prompt, as opposed to a raw shell/vim/
// editor prompt that also reports StatusIdle. Moved here from
// server/services/session_service.go (which now delegates to
// IsSafeSteerStatus) so session/nudge_gate.go's CheckNudgeEligible can share
// the identical allowlist instead of maintaining a second copy that could
// drift — see server/services's
// TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions for the pin
// against claude.go's actual pattern descriptions. insert_mode is
// deliberately excluded: its regex/description aren't distinguishable from a
// real vim INSERT-mode status line.
var SafeIdleStatusContexts = map[string]bool{
	"Claude Code readline input prompt":                                                              true, // claude_readline_prompt
	"Claude Code idle prompt showing ? for shortcuts":                                                true, // claude_shortcuts_prompt
	"Claude Code 'accept edits' review mode — session completed turn, user reviews proposed changes": true, // claude_accept_edits
}

// IsSafeSteerStatus reports whether a detection result is safe for an
// unattended PTY write: StatusIdle with a description on the Claude-specific
// SafeIdleStatusContexts allowlist. StatusIdle alone is NOT sufficient —
// command_prompt/vim_normal_mode/bracket_insert_mode share the same
// DetectedStatus value but mean a raw shell or editor prompt, exactly the
// state where injected text would be misread as a literal command.
//
// StatusSuccess with the turn-complete description is also safe: Claude Code
// v2.1.312 in auto mode shows no "? for shortcuts" footer, so a finished turn's
// "✻ <Verb> for <duration>" marker is the last match in the window. A draft typed
// afterwards, or a newer spinner, is matched first by the reverse line scan and
// takes over from this status.
func IsSafeSteerStatus(status detection.DetectedStatus, statusContext string) bool {
	if status == detection.StatusSuccess {
		return statusContext == TurnCompleteStatusContext
	}
	return status == detection.StatusIdle && SafeIdleStatusContexts[statusContext]
}

// TurnCompleteStatusContext is the verb_duration_completion description in
// session/detection/binaries/claude.go; a test pins it.
const TurnCompleteStatusContext = "Claude turn complete — '<PastTenseVerb> for <duration>' format"
