package detection

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/tstapler/stapler-squad/pkg/ansi"
	"github.com/tstapler/stapler-squad/session/detection/binaries"
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
	"gopkg.in/yaml.v3"
)

// Status represents the current status of a Claude instance based on PTY output analysis.
// This extends the existing Status type in instance.go with additional detection capabilities.
type DetectedStatus int

const (
	StatusUnknown DetectedStatus = iota
	StatusReady
	StatusProcessing
	StatusNeedsApproval
	StatusInputRequired // Explicit user input prompts (questions, "enter X:", etc.)
	StatusError
	StatusTestsFailing    // Tests are failing
	StatusIdle            // Waiting for user input (INSERT mode, command prompt, etc.)
	StatusExecuting       // Actively executing commands (shows "esc to interrupt")
	StatusSuccess         // Task completed successfully
	StatusWaitingForAgent // Waiting for one or more background agents to finish
)

// StatusPattern represents a regex pattern for detecting a specific status.
// This is a type alias for dtypes.StatusPattern to avoid import cycles while
// keeping the type accessible from this package without qualification.
type StatusPattern = dtypes.StatusPattern

// StatusPatterns contains all patterns for status detection.
// This is a type alias for dtypes.StatusPatterns.
type StatusPatterns = dtypes.StatusPatterns

// BinaryDetector provides per-binary pattern sets and optional content filtering.
// This is a type alias for dtypes.BinaryDetector.
type BinaryDetector = dtypes.BinaryDetector

// StatusDetector analyzes PTY output to determine the current status of a Claude instance.
type StatusDetector struct {
	patternSet atomic.Pointer[PatternSet]
	sink       DetectionEventSink
	normalizer PTYNormalizer
}

// NewStatusDetector creates a new status detector with default patterns.
func NewStatusDetector() *StatusDetector {
	ps, _ := NewPatternSet(getDefaultPatterns())
	sd := &StatusDetector{normalizer: PTYNormalizer{}}
	sd.patternSet.Store(ps)
	return sd
}

// validatePatternFilePath rejects paths containing ".." to prevent path traversal.
func validatePatternFilePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("pattern file path rejected (contains '..'): %q", path)
	}
	return nil
}

// NewStatusDetectorFromFile creates a status detector with patterns loaded from a YAML file.
func NewStatusDetectorFromFile(path string) (*StatusDetector, error) {
	if err := validatePatternFilePath(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read status patterns file: %w", err)
	}

	var patterns StatusPatterns
	if err := yaml.Unmarshal(data, &patterns); err != nil {
		return nil, fmt.Errorf("failed to parse status patterns YAML: %w", err)
	}

	ps, err := NewPatternSet(patterns)
	if err != nil {
		return nil, err
	}
	sd := &StatusDetector{normalizer: PTYNormalizer{}}
	sd.patternSet.Store(ps)
	return sd, nil
}

// LoadPatterns loads patterns from a YAML file.
func (sd *StatusDetector) LoadPatterns(path string) error {
	if err := validatePatternFilePath(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read status patterns file: %w", err)
	}

	var patterns StatusPatterns
	if err := yaml.Unmarshal(data, &patterns); err != nil {
		return fmt.Errorf("failed to parse status patterns YAML: %w", err)
	}

	newSet, err := NewPatternSet(patterns)
	if err != nil {
		return err
	}
	sd.patternSet.Store(newSet)
	return nil
}

// ansiStripRegex matches ANSI escape sequences for stripping.
// The three alternatives match:
//  1. CSI sequences:  \x1b[ + digits/semicolons + letter  (colors, cursor moves, etc.)
//  2. OSC sequences:  \x1b] + content + BEL               (window titles, hyperlinks, etc.)
//  3. G0/G1 charset: \x1b( or \x1b) + alphanumeric       (\x1b(B = ASCII, \x1b(0 = graphics)
//
// Modern Claude Code emits \x1b(B (G0 ASCII designator) between each styled character.
// Without rule 3 these remain after stripping and break word-boundary pattern matches
// (e.g. "t\x1b(Bh\x1b(Bi\x1b(Bn\x1b(Bk\x1b(Bi\x1b(Bn\x1b(Bg" never matches "thinking").
// The CSI branch's final-byte class comes from pkg/ansi.CSIFinalByteClass
// (0x40-0x7E per ECMA-48, not just letters) — see that package for why a
// letter-only class is wrong.
var ansiStripRegex = regexp.MustCompile(`\x1b\[[0-9;]*` + ansi.CSIFinalByteClass + `|\x1b\][^\x07]*\x07|\x1b[()][A-Za-z0-9]`)

// cursorForwardRegex matches CSI cursor-right sequences (\x1b[C or \x1b[nC).
// Modern Claude Code uses these instead of literal spaces between words in its status
// bar output (e.g. "esc\x1b[C to\x1b[C interrupt" rather than "esc to interrupt").
// We replace them with a single space before stripping other escapes so that
// word-boundary patterns like `esc\s+to\s+interrupt` continue to match.
var cursorForwardRegex = regexp.MustCompile(`\x1b\[\d*C`)

// stripANSI removes ANSI escape codes from text for cleaner pattern matching.
// Cursor-forward (CSI C) sequences are replaced with a space so that word-separated
// output using terminal cursor positioning still matches whitespace-requiring patterns.
func stripANSI(text string) string {
	text = cursorForwardRegex.ReplaceAllString(text, " ")
	return ansiStripRegex.ReplaceAllString(text, "")
}

// readlineTypingRegex matches the Claude Code readline prompt when the user has
// started composing a message (❯ at column 0 followed by non-digit, non-box-drawing text).
// This distinguishes active user input from:
//   - numbered selection menus (use indented ❯ with a leading space)
//   - horizontal separator lines ("❯ ─────..." using U+2500 BOX DRAWINGS LIGHT HORIZONTAL)
//
// Space matching uses [ \t\x{00a0}] because Claude Code inserts U+00A0 NON-BREAKING SPACE
// between the ❯ cursor and the user's typed text. Regular ASCII space (U+0020) is also
// accepted for compatibility with other tools.
// Checked before Success so a stale ✻ completion marker in scrollback does not override
// the current "user is typing" state.
var readlineTypingRegex = regexp.MustCompile(`(?m)^❯[ \t\x{00a0}]+[^\s\x{00a0}0-9\x{2500}-\x{257F}]`)

// cursorUpRegex matches ANSI cursor-up escape sequences (\x1b[A or \x1b[NA).
var cursorUpRegex = regexp.MustCompile(`\x1b\[\d*A`)

// HasActiveScreenRedraw reports whether raw PTY bytes contain evidence that the
// terminal screen is actively being redrawn: a bare carriage return (not part of
// \r\n) or a cursor-up sequence (\x1b[NA).
func HasActiveScreenRedraw(raw []byte) bool {
	return hasScreenOverwrite(raw)
}

// claudeSpinnerVerbList contains the thinking verbs that Claude Code's spinner
// displays as plaintext bytes in the PTY stream (e.g. "✽ Thinking…", "✦ Analyzing…").
var claudeSpinnerVerbList = []string{
	"thinking", "processing", "analyzing", "working",
	"transmuting", "extracting", "synthesizing", "reasoning",
	"computing", "planning",
}

// HasClaudeSpinnerActivity reports whether tail contains Claude Code's active-thinking
// vocabulary as plaintext. This is more targeted than HasActiveScreenRedraw: cursor-
// positioning sequences appear in both active and idle sessions (from the tmux status
// bar), but spinner verbs only appear when Claude Code is actively running its spinner.
//
// Use as fallback when filterTmuxMetadata has discarded all content (filtered_len == 0)
// or when pattern detection falls through to the Ready catch-all on a single-line tail.
func HasClaudeSpinnerActivity(tail string) bool {
	stripped := strings.ToLower(stripANSI(tail))
	for _, verb := range claudeSpinnerVerbList {
		if strings.Contains(stripped, verb) {
			return true
		}
	}
	return false
}

// hasScreenOverwrite reports whether raw PTY bytes contain evidence of an in-progress
// spinner: a bare carriage return (not part of \r\n, which is a Windows newline)
// or an ANSI cursor-up escape sequence.
// Must be called on the raw output before collapseCarriageReturns() discards this information.
func hasScreenOverwrite(raw []byte) bool {
	for i, b := range raw {
		if b == '\r' {
			// \r\n is a Windows newline — not a screen overwrite
			if i+1 < len(raw) && raw[i+1] == '\n' {
				continue
			}
			return true
		}
	}
	return cursorUpRegex.Match(raw)
}

// collapseCarriageReturns collapses CR-overwritten segments within each line,
// keeping only the final write. "foo\rbar" → "bar"; "\r\n" (Windows newline)
// is treated as a newline boundary and preserved.
func collapseCarriageReturns(s string) string {
	// Fast path: no \r means nothing to collapse — skip Split+Join entirely.
	if strings.IndexByte(s, '\r') < 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// A trailing \r on a line segment is from a \r\n Windows newline; preserve it
		// by re-appending it after collapsing inner CR overwrites.
		trailingCR := strings.HasSuffix(line, "\r")
		if trailingCR {
			line = line[:len(line)-1]
		}
		if strings.IndexByte(line, '\r') >= 0 {
			segments := strings.Split(line, "\r")
			line = segments[len(segments)-1]
		}
		if trailingCR {
			line = line + "\r"
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// StatusDetectionTailBytes is the number of bytes scanned from the tail of terminal
// output when determining session status. Matches DefaultIdleDetectorConfig().BufferSize.
const StatusDetectionTailBytes = 4096

// detectFromText runs all pattern checks on pre-processed text and raw bytes.
// Returns the matched status, the matched pattern's Name (or "" for screen-overwrite,
// "<none>" for no match), and the context description string.
// This is the shared core called by both Detect() and DetectWithContext().
//
// rawPTY must be the original PTY bytes before collapseCarriageReturns is applied.
func (sd *StatusDetector) detectFromText(text string, rawPTY []byte) (DetectedStatus, string, string, int) {
	ps := sd.patternSet.Load()
	return ps.MatchLines(text, rawPTY)
}

// appendDetectionEvent records the outcome of a detection call to the ring buffer.
func (sd *StatusDetector) appendDetectionEvent(status DetectedStatus, patternName, cleanedText string) {
	sd.sink.Record(status, patternName, cleanedText)
}

// SetSessionID sets the session identifier embedded in all future DetectionEvents.
// Call this once after creating the detector, before any detections run.
func (sd *StatusDetector) SetSessionID(id string) {
	sd.sink.SetSessionID(id)
}

// RecentEvents returns up to n most-recent DetectionEvents, newest-first.
func (sd *StatusDetector) RecentEvents(n int) []DetectionEvent {
	return sd.sink.Recent(n)
}

// Detect analyzes the provided PTY output and returns the detected status.
// Patterns are checked in priority order: Error > TestsFailing > Success > NeedsApproval > InputRequired > Active > Processing > Idle > Ready.
// Returns StatusUnknown if no patterns match.
func (sd *StatusDetector) Detect(output []byte) DetectedStatus {
	text := sd.normalizer.Normalize(string(output))
	status, patternName, _, _ := sd.detectFromText(text, output)
	sd.appendDetectionEvent(status, patternName, text)
	return status
}

// DetectWithContext returns the detected status along with a user-friendly context message.
// Uses the pattern's Description field for human-readable messages instead of raw matched text.
func (sd *StatusDetector) DetectWithContext(output []byte) (DetectedStatus, string) {
	text := sd.normalizer.Normalize(string(output))
	status, patternName, context, _ := sd.detectFromText(text, output)
	sd.appendDetectionEvent(status, patternName, text)
	return status, context
}

// detectWithContextFromString is the string-accepting variant of DetectWithContext.
// Avoids the string→[]byte→string round-trip in detectFromLines by aliasing the
// string data via unsafe.Slice for the rawPTY argument (read-only use in hasScreenOverwrite).
// The 3rd return value is the subagent/shell/monitor count captured from the winning
// WaitingForAgent match (0 for any other status) — see MatchLines.
func (sd *StatusDetector) detectWithContextFromString(line string) (DetectedStatus, string, int) {
	text := sd.normalizer.Normalize(line)
	var rawPTY []byte
	if len(line) > 0 {
		rawPTY = unsafe.Slice(unsafe.StringData(line), len(line))
	}
	status, patternName, context, count := sd.detectFromText(text, rawPTY)
	sd.appendDetectionEvent(status, patternName, text)
	return status, context, count
}

// getDefaultPatterns returns the default status detection patterns used as
// the generic fallback for any program with no registered per-binary
// detector (see NewStatusDetector). It delegates to
// binaries.NewClaudeDetector().Patterns() rather than holding its own copy:
// this package already imports session/detection/binaries (registry.go
// registers binaries.NewClaudeDetector() as the built-in "claude" detector),
// so binaries.ClaudeDetector.Patterns() is the single source of truth for
// Claude's default pattern set. Maintaining two copies previously let the
// built-in registry entry (binaries/claude.go) drift into a stale, thinner
// subset of this generic fallback — see that method's doc comment.
func getDefaultPatterns() StatusPatterns {
	return binaries.NewClaudeDetector().Patterns()
}

// StatusString converts DetectedStatus to a human-readable string.
func (s DetectedStatus) String() string {
	switch s {
	case StatusReady:
		return "Ready"
	case StatusProcessing:
		return "Processing"
	case StatusNeedsApproval:
		return "Needs Approval"
	case StatusInputRequired:
		return "Input Required"
	case StatusError:
		return "Error"
	case StatusTestsFailing:
		return "Tests Failing"
	case StatusIdle:
		return "Idle"
	case StatusExecuting:
		return "Executing"
	case StatusSuccess:
		return "Success"
	case StatusWaitingForAgent:
		return "Waiting for Agent"
	case StatusUnknown:
		return "Unknown"
	}
	return "Unknown"
}

// ExportPatterns exports the current patterns to a YAML file.
func (sd *StatusDetector) ExportPatterns(path string) error {
	if err := validatePatternFilePath(path); err != nil {
		return err
	}
	p := sd.patternSet.Load().Patterns()
	data, err := yaml.Marshal(&p)
	if err != nil {
		return fmt.Errorf("failed to marshal status patterns: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write status patterns file: %w", err)
	}

	return nil
}

// GetPatternNames returns the names of all loaded patterns for a given status.
func (sd *StatusDetector) GetPatternNames(status DetectedStatus) []string {
	p := sd.patternSet.Load().Patterns()
	var patterns []StatusPattern
	switch status {
	case StatusReady:
		patterns = p.Ready
	case StatusProcessing:
		patterns = p.Processing
	case StatusNeedsApproval:
		patterns = p.NeedsApproval
	case StatusInputRequired:
		patterns = p.InputRequired
	case StatusError:
		patterns = p.Error
	case StatusTestsFailing:
		patterns = p.TestsFailing
	case StatusIdle:
		patterns = p.Idle
	case StatusExecuting:
		patterns = p.Active
	case StatusSuccess:
		patterns = p.Success
	case StatusWaitingForAgent:
		patterns = p.WaitingForAgent
	case StatusUnknown:
		return nil
	}

	names := make([]string, len(patterns))
	for i, pat := range patterns {
		names[i] = pat.Name
	}
	return names
}

// DetectFromString is a convenience method that accepts a string instead of []byte.
func (sd *StatusDetector) DetectFromString(output string) DetectedStatus {
	return sd.Detect([]byte(output))
}

// DetectForProgram detects the status for output from a named program.
// When the program has a registered BinaryDetector, its per-binary pattern set
// is consulted first. If the per-binary detector returns StatusUnknown (no match),
// the generic Detect() is called as a fallback. For unregistered programs, only
// the generic Detect() is used.
func (sd *StatusDetector) DetectForProgram(output []byte, program string) DetectedStatus {
	if bsd, ok := lookupBinaryDetector(program); ok {
		text := stripANSI(collapseCarriageReturns(string(output)))
		status, patternName, _, _ := bsd.detectFromText(text, output)
		if status != StatusUnknown {
			sd.appendDetectionEvent(status, patternName, text)
			return status
		}
	}
	return sd.Detect(output)
}

// crSegmentScanResult is the outcome of scanning one \r-split line's segments in
// scanCRSegments. When terminal is true, the caller must return (status, desc, count)
// immediately — a definitive, higher-urgency match was found. Otherwise status/desc/count
// carry the (possibly unchanged) best-candidate triple for the caller to fold back in.
type crSegmentScanResult struct {
	terminal bool
	status   DetectedStatus
	desc     string
	count    int
}

// scanCRSegments scans the \r-split segments of a single terminal line in reverse order
// (most recent segment first), applying the same urgency rules detectFromLines uses across
// whole lines. bestStatus is the best candidate found so far on earlier (higher) lines;
// it is only consulted, never assumed non-empty, so callers must fold non-terminal results
// back in themselves (see detectFromLines).
//
// The last segment is always authoritative. Earlier segments: only promote high-urgency
// statuses (Active, NeedsApproval, InputRequired, Error) — these represent session states
// that can be visually hidden by a TUI overlay writing via \r but still indicate the session
// needs attention. Low-urgency statuses (Success, Processing, Idle) in earlier segments were
// overwritten and should not override the visual display.
func (sd *StatusDetector) scanCRSegments(line string, bestStatus DetectedStatus) crSegmentScanResult {
	segs := strings.Split(line, "\r")
	result := crSegmentScanResult{status: bestStatus}
	for j := len(segs) - 1; j >= 0; j-- {
		if strings.TrimSpace(segs[j]) == "" {
			continue
		}
		s, desc, count := sd.detectWithContextFromString(segs[j])
		if s == StatusUnknown {
			continue
		}
		if s == StatusReady {
			if result.status == StatusUnknown {
				result.status, result.desc, result.count = StatusReady, desc, count
			}
			continue
		}
		if j == len(segs)-1 || s == StatusExecuting || s == StatusNeedsApproval || s == StatusInputRequired || s == StatusError {
			return crSegmentScanResult{terminal: true, status: s, desc: desc, count: count}
		}
		// Low-urgency earlier segment: record as candidate but keep scanning.
		if result.status == StatusUnknown {
			result.status, result.desc, result.count = s, desc, count
		}
	}
	return result
}

// detectFromLines is the shared implementation for DetectFromLines and DetectWithContextFromLines.
// Scans lines in reverse (most recent first), handling CR-split segments.
// See DetectFromLines for the full algorithm documentation.
func (sd *StatusDetector) detectFromLines(lines []string) (DetectedStatus, string, int) {
	bestStatus := StatusUnknown
	bestDesc := ""
	bestCount := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if strings.ContainsRune(lines[i], '\r') {
			wasUnknown := bestStatus == StatusUnknown
			res := sd.scanCRSegments(lines[i], bestStatus)
			if res.terminal {
				return res.status, res.desc, res.count
			}
			if wasUnknown && res.status != StatusUnknown {
				bestStatus, bestDesc, bestCount = res.status, res.desc, res.count
			}
			continue // all segments of this CR line handled above
		}

		s, desc, count := sd.detectWithContextFromString(lines[i])
		if s == StatusUnknown {
			continue
		}
		if s == StatusReady {
			if bestStatus == StatusUnknown {
				bestStatus, bestDesc, bestCount = StatusReady, desc, count
			}
			continue
		}
		// StatusExecuting: store as candidate and keep scanning upward.
		// WaitingForAgent is more specific (higher priority in single-line matching)
		// and often appears on the spinner line above the "esc to interrupt" status bar.
		// High-urgency statuses (Error, NeedsApproval, InputRequired) also override Active.
		if s == StatusExecuting {
			if bestStatus == StatusUnknown || bestStatus == StatusReady {
				bestStatus, bestDesc, bestCount = StatusExecuting, desc, count
			}
			continue
		}
		// If we already have Executing stored, only accept higher-urgency statuses from
		// earlier (higher) lines. Success/Processing/Idle on earlier lines are stale.
		if bestStatus == StatusExecuting {
			switch s {
			case StatusWaitingForAgent, StatusError, StatusNeedsApproval, StatusInputRequired:
				return s, desc, count
			case StatusUnknown, StatusReady, StatusProcessing, StatusIdle, StatusSuccess, StatusTestsFailing, StatusExecuting:
				// Lower-urgency statuses when we already have Executing — skip
				continue
			}
			continue
		}
		return s, desc, count // specific match wins immediately
	}
	return bestStatus, bestDesc, bestCount
}

// DetectFromLines analyzes multiple lines of output and returns the most relevant status.
// Lines are processed in reverse order (most recent first) so the current terminal
// state takes precedence over stale scrollback content.
//
// Blank/whitespace-only lines are skipped. StatusReady results are noted but the scan
// continues looking for a more specific status — this prevents the `.*` Ready catch-all
// from stopping the scan on an unrelated line (e.g. "PR #66") before reaching a real
// status pattern on an earlier line. StatusReady is returned as a fallback if no more
// specific status is found.
func (sd *StatusDetector) DetectFromLines(lines []string) DetectedStatus {
	s, _, _ := sd.detectFromLines(lines)
	return s
}

// DetectWithContextFromLines analyzes lines in reverse order (most recent first) and returns
// the detected status with context. This ensures current terminal state (e.g. "? for shortcuts"
// on the last line) takes precedence over stale scrollback content (e.g. an old "esc to interrupt"
// from a previous turn that is still within the scanned window).
//
// Blank/whitespace-only lines are skipped. StatusReady is treated as a low-confidence
// fallback — the scan continues past Ready results looking for a more specific status,
// preventing the `.*` catch-all from masking real patterns on earlier lines.
//
// Signature intentionally left unchanged (pinned by the TerminalDetector interface and its
// callers) — see DetectWithContextAndCountFromLines for the count-aware sibling.
func (sd *StatusDetector) DetectWithContextFromLines(lines []string) (DetectedStatus, string) {
	s, desc, _ := sd.detectFromLines(lines)
	return s, desc
}

// DetectWithContextAndCountFromLines is the count-aware sibling of DetectWithContextFromLines.
// It returns the same status and context, plus the subagent/shell/monitor count captured
// from the winning WaitingForAgent match (0 for any other status). Added as a new method
// rather than changing DetectWithContextFromLines in place because that method is pinned by
// the TerminalDetector interface and consumed by review_queue_determiner.go plus several
// test files that don't need the count.
func (sd *StatusDetector) DetectWithContextAndCountFromLines(lines []string) (DetectedStatus, string, int) {
	return sd.detectFromLines(lines)
}

// DetectRecent analyzes the most recent n bytes of output for status detection.
// This is optimized for real-time status monitoring.
func (sd *StatusDetector) DetectRecent(output []byte, n int) DetectedStatus {
	if n <= 0 || len(output) == 0 {
		return StatusUnknown
	}

	startPos := len(output) - n
	if startPos < 0 {
		startPos = 0
	}

	return sd.Detect(output[startPos:])
}

// HasPattern checks if a specific pattern name exists for the given status.
func (sd *StatusDetector) HasPattern(status DetectedStatus, name string) bool {
	patterns := sd.GetPatternNames(status)
	for _, p := range patterns {
		if strings.EqualFold(p, name) {
			return true
		}
	}
	return false
}
