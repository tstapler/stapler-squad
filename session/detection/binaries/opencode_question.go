package binaries

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

// OpenCode 1.18.30 permission dialog (Spike S5, fixture
// session/testdata/panes/opencode/permission_bash.txt): a `┃ △ Permission
// required` header, the request, and a button row `Allow once  Allow always
// Reject`. Only Enter on the default button (Allow once) was measured; digits
// do nothing, and the other two keystrokes are unmeasured, so they are not offered.
const (
	openCodeProfileName    = "opencode"
	openCodeHeader         = "△ Permission required"
	openCodeHeaderLookback = 8  // header must be within N raw lines above the button row
	openCodeLeftColumns    = 80 // a sidebar is drawn at column ~176 on wide panes
	openCodeEnter          = "\r"
)

var (
	openCodeAnsi    = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][A-Za-z0-9]`)
	openCodeButtons = regexp.MustCompile(`\s{2,}`)
)

// OpenCodeQuestionProfile is the replyable profile of OpenCode's permission dialog.
type OpenCodeQuestionProfile struct{}

// NewOpenCodeQuestionProfile returns the OpenCode profile.
func NewOpenCodeQuestionProfile() OpenCodeQuestionProfile { return OpenCodeQuestionProfile{} }

// Name returns "opencode".
func (OpenCodeQuestionProfile) Name() string { return openCodeProfileName }

type openCodeLine struct {
	text  string // left pane only, bar and surrounding space removed
	empty bool   // nothing but whitespace or the bare bar
}

func openCodeLines(capture string) []openCodeLine {
	raw := strings.Split(capture, "\n")
	out := make([]openCodeLine, len(raw))
	for i, l := range raw {
		l = openCodeAnsi.ReplaceAllString(l, "")
		if r := []rune(l); len(r) > openCodeLeftColumns {
			l = string(r[:openCodeLeftColumns])
		}
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "┃"))
		out[i] = openCodeLine{text: strings.TrimRightFunc(l, unicode.IsSpace), empty: l == ""}
	}
	return out
}

// Extract requires the button row to be the last non-empty line (a sidebar or
// bare bar does not count), the header above it and the request between them.
// Any row after the buttons, such as a forged `1. Allow once`, is no match.
func (OpenCodeQuestionProfile) Extract(capture string) (dtypes.ProfileDialog, bool) {
	lines := openCodeLines(capture)
	last := len(lines) - 1
	for last >= 0 && lines[last].empty {
		last--
	}
	if last < 0 {
		return dtypes.ProfileDialog{}, false
	}
	opts, ok := openCodeButtonRow(lines[last].text)
	if !ok {
		return dtypes.ProfileDialog{}, false
	}
	return openCodeDialog(lines, last, opts)
}

func openCodeButtonRow(text string) ([]string, bool) {
	if !strings.HasPrefix(text, "Allow once") {
		return nil, false
	}
	opts := openCodeButtons.Split(text, -1)
	if len(opts) != 3 || opts[0] != "Allow once" || opts[1] != "Allow always" || opts[2] != "Reject" {
		return nil, false
	}
	return opts, true
}

func openCodeDialog(lines []openCodeLine, button int, opts []string) (dtypes.ProfileDialog, bool) {
	header := -1
	for i := button - 1; i >= 0 && button-i <= openCodeHeaderLookback; i-- {
		if lines[i].text == openCodeHeader {
			header = i
			break
		}
	}
	if header < 0 {
		return dtypes.ProfileDialog{}, false
	}
	var q []string
	for _, l := range lines[header+1 : button] {
		if !l.empty {
			q = append(q, strings.Join(strings.Fields(l.text), " "))
		}
	}
	return dtypes.ProfileDialog{
		Profile: openCodeProfileName, Header: openCodeHeader,
		Question: strings.Join(q, " "), Options: opts,
	}, true
}

// Verify re-extracts and requires the same request and buttons.
func (p OpenCodeQuestionProfile) Verify(capture string, want dtypes.ProfileDialog) bool {
	got, ok := p.Extract(capture)
	if !ok || got.Question != want.Question || len(got.Options) != len(want.Options) {
		return false
	}
	for i := range got.Options {
		if got.Options[i] != want.Options[i] {
			return false
		}
	}
	return true
}

// AnswerKeys: only Allow once (the default button) has a measured keystroke.
func (OpenCodeQuestionProfile) AnswerKeys(optionIndex int, want dtypes.ProfileDialog) (string, bool) {
	if optionIndex == 0 && len(want.Options) > 0 && want.Options[0] == "Allow once" {
		return openCodeEnter, true
	}
	return "", false
}
