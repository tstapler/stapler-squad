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
	compactingRegexes      []*regexp.Regexp
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
		{"compacting", ps.patterns.Compacting, &ps.compactingRegexes},
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

// matchFirst scans regexes in order for the first match against text and returns status
// alongside the matching pattern's name/description. ok is false when nothing matched, in
// which case the other return values are zero values and must be ignored.
//
// Extracted from MatchLines' priority chain — every group in that chain except
// WaitingForAgent (which also needs the capture-group subagent count) follows this exact
// "first match wins" shape, so factoring it out keeps MatchLines' branching within lint's
// complexity gate instead of repeating a for+if pair per group.
func matchFirst(regexes []*regexp.Regexp, patterns []StatusPattern, status DetectedStatus, text string) (DetectedStatus, string, string, bool) {
	for i, regex := range regexes {
		if regex.MatchString(text) {
			return status, patterns[i].Name, patterns[i].Description, true
		}
	}
	return StatusUnknown, "", "", false
}

// matchWaitingForAgent scans the WaitingForAgent regexes and, on a match, extracts the
// subagent/shell/monitor count from the pattern's capture group. See the capturing group on
// the three WaitingForAgent patterns in binaries.ClaudeDetector.Patterns()
// (session/detection/binaries/claude.go), the single source of truth delegated to by
// getDefaultPatterns().
func (ps *PatternSet) matchWaitingForAgent(text string) (name, desc string, count int, ok bool) {
	for i, regex := range ps.waitingForAgentRegexes {
		m := regex.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		// len(m) > 1 guard is defensive insurance against a future pattern edit
		// dropping/reordering the capture group; today's 3 patterns always produce
		// len(m) == 2 on match (idiom from session/git/worktree_git.go:372-375).
		if len(m) > 1 {
			if n, convErr := strconv.Atoi(m[1]); convErr == nil && n > 0 {
				count = n
			}
		}
		return ps.patterns.WaitingForAgent[i].Name, ps.patterns.WaitingForAgent[i].Description, count, true
	}
	return "", "", 0, false
}

// MatchLines runs the pattern priority chain on the given text string and raw PTY bytes.
// Returns (status, patternName, description, subagentCount). subagentCount is only ever
// non-zero when the WaitingForAgent group wins — see matchWaitingForAgent.
func (ps *PatternSet) MatchLines(text string, rawPTY []byte) (DetectedStatus, string, string, int) {
	// Error patterns (highest priority)
	if s, name, desc, ok := matchFirst(ps.errorRegexes, ps.patterns.Error, StatusError, text); ok {
		return s, name, desc, 0
	}
	// Tests failing
	if s, name, desc, ok := matchFirst(ps.testsFailingRegexes, ps.patterns.TestsFailing, StatusTestsFailing, text); ok {
		return s, name, desc, 0
	}
	// Needs approval
	if s, name, desc, ok := matchFirst(ps.needsApprovalRegexes, ps.patterns.NeedsApproval, StatusNeedsApproval, text); ok {
		return s, name, desc, 0
	}
	// Input required
	if s, name, desc, ok := matchFirst(ps.inputRequiredRegexes, ps.patterns.InputRequired, StatusInputRequired, text); ok {
		return s, name, desc, 0
	}
	// Readline typing
	if readlineTypingRegex.MatchString(text) {
		return StatusIdle, "readline_typing", "User composing at Claude readline — overrides stale completion marker in scrollback", 0
	}
	// Waiting for background agent/shells — checked BEFORE Success so that a line like
	// "✻ Churned for 52s · 1 shell still running" is classified as WaitingForAgent
	// rather than Success (the verb-duration pattern in Success would otherwise win).
	if name, desc, count, ok := ps.matchWaitingForAgent(text); ok {
		return StatusWaitingForAgent, name, desc, count
	}
	// Success
	if s, name, desc, ok := matchFirst(ps.successRegexes, ps.patterns.Success, StatusSuccess, text); ok {
		return s, name, desc, 0
	}
	// Compacting — checked BEFORE Active so a "Compacting conversation" line is not
	// swallowed by esc_to_interrupt or claude_thinking_verb (both would otherwise
	// classify it as generic Active/Executing — see claude.go's compacting_conversation
	// pattern comment for the exact regex overlap this avoids). WARNING: reordering
	// this loop to after Active/Processing makes StatusCompacting unreachable.
	for i, regex := range ps.compactingRegexes {
		if regex.MatchString(text) {
			return StatusCompacting, ps.patterns.Compacting[i].Name, ps.patterns.Compacting[i].Description, 0
		}
	}
	// Active
	if s, name, desc, ok := matchFirst(ps.activeRegexes, ps.patterns.Active, StatusExecuting, text); ok {
		return s, name, desc, 0
	}
	// Processing
	if s, name, desc, ok := matchFirst(ps.processingRegexes, ps.patterns.Processing, StatusProcessing, text); ok {
		return s, name, desc, 0
	}
	// Screen-overwrite fallback
	if hasScreenOverwrite(rawPTY) {
		return StatusExecuting, "screen_overwrite", "Screen overwrite — spinner actively redrawing", 0
	}
	// Idle
	if s, name, desc, ok := matchFirst(ps.idleRegexes, ps.patterns.Idle, StatusIdle, text); ok {
		return s, name, desc, 0
	}
	// Ready (catch-all — must be last; returns StatusUnknown so the .* pattern renders no badge)
	if _, name, desc, ok := matchFirst(ps.readyRegexes, ps.patterns.Ready, StatusUnknown, text); ok {
		return StatusUnknown, name, desc, 0
	}
	return StatusUnknown, "<none>", "", 0
}

// Patterns returns the StatusPatterns used by this PatternSet.
func (ps *PatternSet) Patterns() StatusPatterns {
	return ps.patterns
}
