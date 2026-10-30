package session

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SignalKind is how a program's end of turn can be observed.
type SignalKind string

const (
	SignalHook    SignalKind = "hook"       // the program fires a Stop hook
	SignalScreen  SignalKind = "screen"     // read the idle prompt and the prose above it
	SignalIdleFor SignalKind = "idle-for-N" // only output silence for N is observable
	SignalNone    SignalKind = "none"       // no usable signal: the card says "cannot detect"
)

// ProgramSignal is one row of ProgramSignalTable.
type ProgramSignal struct {
	Kind SignalKind
	// IdleContexts is the exact set of detection.StatusIdle descriptions that mean "this
	// program's own idle prompt" (screen fallback gate). Each entry is pinned against the
	// program's binaries/*.go pattern descriptions.
	IdleContexts map[string]bool
	// PromptRE matches the program's input-prompt line when its detector has no Idle
	// pattern to gate on (aider). The last non-blank pane line must match.
	PromptRE *regexp.Regexp
	// IdleFor is N for SignalIdleFor rows.
	IdleFor time.Duration
	// Note says why the row is what it is.
	Note string
}

// ScreenCapable reports whether the screen fallback stage can run for the row.
func (s ProgramSignal) ScreenCapable() bool {
	return len(s.IdleContexts) > 0 || s.PromptRE != nil
}

// agyIdleReadlineContext is agy_idle_readline's description in binaries/agy.go; a test pins it.
// agy_idle_insert is excluded: "[INSERT]" is indistinguishable from a vim status line.
const agyIdleReadlineContext = "Agy readline input cursor on empty line (shared with Gemini idle TUI)"

// agyIdleContexts: the default detector merges every program's patterns, so an agy pane can
// come back with Claude's readline description instead of agy's own.
func agyIdleContexts() map[string]bool {
	out := map[string]bool{agyIdleReadlineContext: true}
	for ctx := range SafeIdleStatusContexts {
		out[ctx] = true
	}
	return out
}

// aiderPromptRE matches aider's input prompt ("> ", "ask> ", "architect> ").
var aiderPromptRE = regexp.MustCompile(`^(?:[a-z]+)?> ?$`)

// ProgramSignalTable is the honest per-program coverage of turn-end detection.
var ProgramSignalTable = map[string]ProgramSignal{
	"claude": {
		Kind:         SignalHook,
		IdleContexts: SafeIdleStatusContexts,
		Note:         "Stop hook is primary; the screen stage covers sessions whose hook never fires",
	},
	"agy": {
		Kind:         SignalScreen,
		IdleContexts: agyIdleContexts(),
		Note:         "Antigravity: idle readline cursor; the shared detector may report it with Claude's readline context",
	},
	"aider": {
		Kind:     SignalScreen,
		PromptRE: aiderPromptRE,
		Note:     "no Idle pattern in binaries/aider.go; gated on the prompt line itself",
	},
	"gemini":   {Kind: SignalNone, Note: "binaries/gemini.go has no Idle or Success pattern"},
	"opencode": {Kind: SignalNone, Note: "binaries/opencode.go has no distinctive idle string"},
	"pi":       {Kind: SignalNone, Note: "status comes from a side-channel subprocess, not pane text"},
}

// normalizeProgram maps a launch command ("/usr/bin/claude --resume") to its table key.
func normalizeProgram(program string) string {
	fields := strings.Fields(program)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(filepath.Base(fields[0]))
}

// ProgramSignalFor returns the row for program; an unknown program is SignalNone.
func ProgramSignalFor(program string) ProgramSignal {
	if row, ok := ProgramSignalTable[normalizeProgram(program)]; ok {
		return row
	}
	return ProgramSignal{Kind: SignalNone, Note: "unknown program"}
}

// CannotDetect reports that a program's turn end is invisible, so the session card shows
// "cannot detect" instead of nothing.
func CannotDetect(program string) bool {
	return ProgramSignalFor(program).Kind == SignalNone
}
