package detection

import (
	"fmt"
	"regexp"
	"strconv"
)

// PatternSet holds compiled regex slices for all StatusPatterns categories.
// Immutable after NewPatternSet returns — no lock needed.
type PatternSet struct {
	patterns StatusPatterns

	readyRegexes           []*regexp.Regexp
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
	return nil
}

// MatchLines runs the pattern priority chain on the given text string and raw PTY bytes.
// Returns (status, patternName, description, subagentCount). subagentCount is only ever
// non-zero when the WaitingForAgent group wins — see the capturing group on the three
// WaitingForAgent patterns in binaries.ClaudeDetector.Patterns() (session/detection/binaries/claude.go),
// the single source of truth delegated to by getDefaultPatterns().
func (ps *PatternSet) MatchLines(text string, rawPTY []byte) (DetectedStatus, string, string, int) {
	// Error patterns (highest priority)
	for i, regex := range ps.errorRegexes {
		if regex.MatchString(text) {
			return StatusError, ps.patterns.Error[i].Name, ps.patterns.Error[i].Description, 0
		}
	}
	// Tests failing
	for i, regex := range ps.testsFailingRegexes {
		if regex.MatchString(text) {
			return StatusTestsFailing, ps.patterns.TestsFailing[i].Name, ps.patterns.TestsFailing[i].Description, 0
		}
	}
	// Needs approval
	for i, regex := range ps.needsApprovalRegexes {
		if regex.MatchString(text) {
			return StatusNeedsApproval, ps.patterns.NeedsApproval[i].Name, ps.patterns.NeedsApproval[i].Description, 0
		}
	}
	// Input required
	for i, regex := range ps.inputRequiredRegexes {
		if regex.MatchString(text) {
			return StatusInputRequired, ps.patterns.InputRequired[i].Name, ps.patterns.InputRequired[i].Description, 0
		}
	}
	// Readline typing
	if readlineTypingRegex.MatchString(text) {
		return StatusIdle, "readline_typing", "User composing at Claude readline — overrides stale completion marker in scrollback", 0
	}
	// Waiting for background agent/shells — checked BEFORE Success so that a line like
	// "✻ Churned for 52s · 1 shell still running" is classified as WaitingForAgent
	// rather than Success (the verb-duration pattern in Success would otherwise win).
	for i, regex := range ps.waitingForAgentRegexes {
		if m := regex.FindStringSubmatch(text); m != nil {
			count := 0
			// len(m) > 1 guard is defensive insurance against a future pattern edit
			// dropping/reordering the capture group; today's 3 patterns always produce
			// len(m) == 2 on match (idiom from session/git/worktree_git.go:372-375).
			if len(m) > 1 {
				if n, convErr := strconv.Atoi(m[1]); convErr == nil && n > 0 {
					count = n
				}
			}
			return StatusWaitingForAgent, ps.patterns.WaitingForAgent[i].Name, ps.patterns.WaitingForAgent[i].Description, count
		}
	}
	// Success
	for i, regex := range ps.successRegexes {
		if regex.MatchString(text) {
			return StatusSuccess, ps.patterns.Success[i].Name, ps.patterns.Success[i].Description, 0
		}
	}
	// Active
	for i, regex := range ps.activeRegexes {
		if regex.MatchString(text) {
			return StatusExecuting, ps.patterns.Active[i].Name, ps.patterns.Active[i].Description, 0
		}
	}
	// Processing
	for i, regex := range ps.processingRegexes {
		if regex.MatchString(text) {
			return StatusProcessing, ps.patterns.Processing[i].Name, ps.patterns.Processing[i].Description, 0
		}
	}
	// Screen-overwrite fallback
	if hasScreenOverwrite(rawPTY) {
		return StatusExecuting, "screen_overwrite", "Screen overwrite — spinner actively redrawing", 0
	}
	// Idle
	for i, regex := range ps.idleRegexes {
		if regex.MatchString(text) {
			return StatusIdle, ps.patterns.Idle[i].Name, ps.patterns.Idle[i].Description, 0
		}
	}
	// Ready (catch-all — must be last; returns StatusUnknown so the .* pattern renders no badge)
	for i, regex := range ps.readyRegexes {
		if regex.MatchString(text) {
			return StatusUnknown, ps.patterns.Ready[i].Name, ps.patterns.Ready[i].Description, 0
		}
	}
	return StatusUnknown, "<none>", "", 0
}

// Patterns returns the StatusPatterns used by this PatternSet.
func (ps *PatternSet) Patterns() StatusPatterns {
	return ps.patterns
}
