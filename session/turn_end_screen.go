package session

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/session/detection"
)

// ScreenTurnEnd is the screen fallback's reading of a finished turn.
type ScreenTurnEnd struct {
	Text          string // the bottom-most prose paragraph
	Kind          TurnEndKind
	Confidence    TurnEndConfidence
	NeedsTiebreak bool // ambiguous Finished a later LLM tiebreak may upgrade
}

// DetectScreenTurnEnd reads a turn end off the pane when the program is idle at its own
// prompt. The gate is detection.StatusIdle plus the program's idle-context allowlist (or its
// prompt line), never detection.StatusReady, which no pattern produces. Non-Claude programs
// yield Finished until they have a labelled corpus (ClassifyTurnEnd enforces that).
func DetectScreenTurnEnd(program string, status detection.DetectedStatus, statusContext, content string) (ScreenTurnEnd, bool) {
	row := ProgramSignalFor(program)
	if !row.ScreenCapable() {
		return ScreenTurnEnd{}, false
	}
	lines := strings.Split(content, "\n")
	if !screenIdleGate(row, status, statusContext, lines) {
		return ScreenTurnEnd{}, false
	}
	text := bottomProseParagraph(lines)
	if text == "" {
		return ScreenTurnEnd{}, false
	}
	cls := ClassifyTurnEnd(program, text)
	out := ScreenTurnEnd{Text: text, Kind: TurnEndKindFinished, Confidence: cls.Confidence, NeedsTiebreak: cls.NeedsTiebreak()}
	if cls.Class == TurnEndClassQuestion && cls.Confidence == TurnEndConfidenceHigh {
		out.Kind = TurnEndKindQuestion
	}
	return out, true
}

func screenIdleGate(row ProgramSignal, status detection.DetectedStatus, statusContext string, lines []string) bool {
	if row.Kind == SignalHook {
		return IsSafeSteerStatus(status, statusContext)
	}
	if row.PromptRE != nil {
		last := lastNonBlank(lines)
		return last != "" && row.PromptRE.MatchString(last)
	}
	return status == detection.StatusIdle && row.IdleContexts[statusContext]
}

func lastNonBlank(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(stripANSI([]byte(lines[i]))); l != "" {
			return l
		}
	}
	return ""
}

// chromeMarkers are footer/status lines that sit between the prose and the prompt.
var chromeMarkers = []string{"? for shortcuts", "accept edits", "esc to interrupt", "bypass permissions", "tokens:", "cost:"}

// isChromeLine reports a line that is not prose: borders, prompts, footers, the turn-complete marker.
func isChromeLine(l string) bool {
	if l == "" {
		return true
	}
	if first, _ := utf8.DecodeRuneInString(l); strings.ContainsRune(">│╭╰┌└✻⏵", first) {
		return true
	}
	if strings.IndexFunc(l, unicode.IsLetter) < 0 {
		return true // rules, bare symbols
	}
	low := strings.ToLower(l)
	for _, m := range chromeMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}

var proseBullets = []string{"⏺ ", "● ", "• "}

// bottomProseParagraph returns the bottom-most run of prose lines (no blank line inside),
// scanning upward from the screen bottom past prompt and footer chrome.
func bottomProseParagraph(lines []string) string {
	clean := make([]string, len(lines))
	for i, l := range lines {
		clean[i] = strings.TrimSpace(stripANSI([]byte(l)))
	}
	end := len(clean) - 1
	for end >= 0 && isChromeLine(clean[end]) {
		end--
	}
	if end < 0 {
		return ""
	}
	start := end
	for start > 0 && clean[start-1] != "" && !isChromeLine(clean[start-1]) {
		start--
	}
	parts := make([]string, 0, end-start+1)
	for _, l := range clean[start : end+1] {
		for _, b := range proseBullets {
			l = strings.TrimPrefix(l, b)
		}
		parts = append(parts, l)
	}
	text := strings.Join(parts, "\n")
	if len(strings.Fields(text)) < 3 {
		return ""
	}
	return text
}
