package detection

import (
	"fmt"
	"regexp"
)

// PatternSet holds compiled regex slices for all StatusPatterns categories.
// Immutable after NewPatternSet returns — no lock needed.
type PatternSet struct {
	patterns StatusPatterns

	readyRegexes           []*regexp.Regexp
	readyCatchAll          []bool // readyCatchAll[i] is true when patterns.Ready[i] is the universal `.*` catch-all
	processingRegexes      []*regexp.Regexp
	needsApprovalRegexes   []*regexp.Regexp
	inputRequiredRegexes   []*regexp.Regexp
	errorRegexes           []*regexp.Regexp
	testsFailingRegexes    []*regexp.Regexp
	idleRegexes            []*regexp.Regexp
	activeRegexes          []*regexp.Regexp
	successRegexes         []*regexp.Regexp
	waitingForAgentRegexes []*regexp.Regexp
}

// NewPatternSet compiles all patterns in p. Returns an error if any regex is invalid.
func NewPatternSet(p StatusPatterns) (*PatternSet, error) {
	ps := &PatternSet{patterns: p}
	if err := ps.compile(); err != nil {
		return nil, err
	}
	return ps, nil
}

// compile compiles all regex patterns. Called at construction time only (no lock needed).
func (ps *PatternSet) compile() error {
	type group struct {
		label    string
		patterns []StatusPattern
		out      *[]*regexp.Regexp
	}
	groups := []group{
		{"ready", ps.patterns.Ready, &ps.readyRegexes},
		{"processing", ps.patterns.Processing, &ps.processingRegexes},
		{"needs_approval", ps.patterns.NeedsApproval, &ps.needsApprovalRegexes},
		{"input_required", ps.patterns.InputRequired, &ps.inputRequiredRegexes},
		{"error", ps.patterns.Error, &ps.errorRegexes},
		{"tests_failing", ps.patterns.TestsFailing, &ps.testsFailingRegexes},
		{"idle", ps.patterns.Idle, &ps.idleRegexes},
		{"active", ps.patterns.Active, &ps.activeRegexes},
		{"success", ps.patterns.Success, &ps.successRegexes},
		{"waiting_for_agent", ps.patterns.WaitingForAgent, &ps.waitingForAgentRegexes},
	}
	for _, g := range groups {
		compiled := make([]*regexp.Regexp, len(g.patterns))
		for i, pat := range g.patterns {
			rx, err := regexp.Compile(pat.Pattern)
			if err != nil {
				return fmt.Errorf("failed to compile %s pattern %q: %w", g.label, pat.Name, err)
			}
			compiled[i] = rx
		}
		*g.out = compiled
	}

	// Precompute which entries in the Ready bucket are the universal `.*` catch-all
	// (e.g. "claude_prompt") versus explicit named ready patterns (e.g. "gemini_ready").
	// The catch-all matches every string, so it must be checked last and separately —
	// otherwise it would shadow explicit ready patterns in the same slice.
	// See project_plans/session-status-unification/implementation/adversarial-review.md
	// Issue 4: StatusReady keeps a distinct definition; StatusUnknown is the `.*` catch-all.
	catchAll := make([]bool, len(ps.patterns.Ready))
	for i, pat := range ps.patterns.Ready {
		catchAll[i] = pat.Pattern == ".*"
	}
	ps.readyCatchAll = catchAll

	return nil
}

// MatchLines runs the pattern priority chain on the given text string and raw PTY bytes.
// Returns (status, patternName, description).
func (ps *PatternSet) MatchLines(text string, rawPTY []byte) (DetectedStatus, string, string) {
	// Error patterns (highest priority)
	for i, regex := range ps.errorRegexes {
		if regex.MatchString(text) {
			return StatusError, ps.patterns.Error[i].Name, ps.patterns.Error[i].Description
		}
	}
	// Tests failing
	for i, regex := range ps.testsFailingRegexes {
		if regex.MatchString(text) {
			return StatusTestsFailing, ps.patterns.TestsFailing[i].Name, ps.patterns.TestsFailing[i].Description
		}
	}
	// Needs approval
	for i, regex := range ps.needsApprovalRegexes {
		if regex.MatchString(text) {
			return StatusNeedsApproval, ps.patterns.NeedsApproval[i].Name, ps.patterns.NeedsApproval[i].Description
		}
	}
	// Input required
	for i, regex := range ps.inputRequiredRegexes {
		if regex.MatchString(text) {
			return StatusInputRequired, ps.patterns.InputRequired[i].Name, ps.patterns.InputRequired[i].Description
		}
	}
	// Readline typing
	if readlineTypingRegex.MatchString(text) {
		return StatusIdle, "readline_typing", "User composing at Claude readline — overrides stale completion marker in scrollback"
	}
	// Waiting for background agent/shells — checked BEFORE Success so that a line like
	// "✻ Churned for 52s · 1 shell still running" is classified as WaitingForAgent
	// rather than Success (the verb-duration pattern in Success would otherwise win).
	for i, regex := range ps.waitingForAgentRegexes {
		if regex.MatchString(text) {
			return StatusWaitingForAgent, ps.patterns.WaitingForAgent[i].Name, ps.patterns.WaitingForAgent[i].Description
		}
	}
	// Success
	for i, regex := range ps.successRegexes {
		if regex.MatchString(text) {
			return StatusSuccess, ps.patterns.Success[i].Name, ps.patterns.Success[i].Description
		}
	}
	// Active
	for i, regex := range ps.activeRegexes {
		if regex.MatchString(text) {
			return StatusExecuting, ps.patterns.Active[i].Name, ps.patterns.Active[i].Description
		}
	}
	// Processing
	for i, regex := range ps.processingRegexes {
		if regex.MatchString(text) {
			return StatusProcessing, ps.patterns.Processing[i].Name, ps.patterns.Processing[i].Description
		}
	}
	// Screen-overwrite fallback
	if hasScreenOverwrite(rawPTY) {
		return StatusExecuting, "screen_overwrite", "Screen overwrite — spinner actively redrawing"
	}
	// Idle
	for i, regex := range ps.idleRegexes {
		if regex.MatchString(text) {
			return StatusIdle, ps.patterns.Idle[i].Name, ps.patterns.Idle[i].Description
		}
	}
	// Ready — explicit named patterns (e.g. gemini_ready) are checked first and return
	// StatusReady. These are distinct from the universal `.*` catch-all in the same
	// bucket and must be checked before it, or the catch-all would always win.
	for i, regex := range ps.readyRegexes {
		if ps.readyCatchAll[i] {
			continue
		}
		if regex.MatchString(text) {
			return StatusReady, ps.patterns.Ready[i].Name, ps.patterns.Ready[i].Description
		}
	}
	// Ready catch-all (must be last; returns StatusUnknown so the .* pattern renders no badge)
	for i, regex := range ps.readyRegexes {
		if !ps.readyCatchAll[i] {
			continue
		}
		if regex.MatchString(text) {
			return StatusUnknown, ps.patterns.Ready[i].Name, ps.patterns.Ready[i].Description
		}
	}
	return StatusUnknown, "<none>", ""
}

// Patterns returns the StatusPatterns used by this PatternSet.
func (ps *PatternSet) Patterns() StatusPatterns {
	return ps.patterns
}
