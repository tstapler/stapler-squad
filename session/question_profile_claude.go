package session

import (
	"strings"

	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/detection/binaries"
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

// claudeProfileName is the profile id of Claude Code's AskUserQuestion dialog.
const claudeProfileName = "claude"

// ClaudeQuestionProfile is the single-select AskUserQuestion profile. It lives
// in this package, not session/detection, because it wraps DialogMatch and
// detection cannot import session.
type ClaudeQuestionProfile struct{}

// Name returns "claude".
func (ClaudeQuestionProfile) Name() string { return claudeProfileName }

func profileFromQuestion(q QuestionDialog) dtypes.ProfileDialog {
	return dtypes.ProfileDialog{Profile: claudeProfileName, Header: q.Header, Question: q.Question, Options: q.Labels}
}

// Extract reads the question off the capture and keeps it only when the existing
// DialogMatch accepts that same question, so extraction can never be looser than
// the pre-write guard.
func (ClaudeQuestionProfile) Extract(capture string) (dtypes.ProfileDialog, bool) {
	lines := dialogLines(capture)
	footer := lastFooterIndex(lines)
	if footer < 0 {
		return dtypes.ProfileDialog{}, false
	}
	header := -1
	for i := footer - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], dialogHeaderGlyph) {
			header = i
			break
		}
	}
	if header < 0 {
		return dtypes.ProfileDialog{}, false
	}
	question, rows, ok := parseDialogRegion(lines[header+1 : footer])
	if !ok || len(rows) < 3 {
		return dtypes.ProfileDialog{}, false
	}
	labels := make([]string, 0, len(rows)-2)
	for _, r := range rows[:len(rows)-2] {
		labels = append(labels, r.text)
	}
	q := QuestionDialog{Question: question, Header: strings.TrimPrefix(lines[header], dialogHeaderGlyph), Labels: labels}
	if !DialogMatch(capture, q) {
		return dtypes.ProfileDialog{}, false
	}
	return profileFromQuestion(q), true
}

// Verify is DialogMatch on the wanted question.
func (ClaudeQuestionProfile) Verify(capture string, want dtypes.ProfileDialog) bool {
	return DialogMatch(capture, QuestionDialog{Question: want.Question, Header: want.Header, Labels: want.Options})
}

// AnswerKeys is the option's digit: a digit selects immediately, with no Enter.
func (ClaudeQuestionProfile) AnswerKeys(optionIndex int, want dtypes.ProfileDialog) (string, bool) {
	if optionIndex < 0 || optionIndex >= len(want.Options) || optionIndex > 8 {
		return "", false
	}
	return string(rune('1' + optionIndex)), true
}

// DefaultQuestionProfiles is the registry of replyable harnesses: exactly the
// ones Spike S5 measured. Aider (reply keystroke unverified), pi, Gemini and
// agy are deliberately absent and stay read_only.
func DefaultQuestionProfiles() *detection.QuestionProfileRegistry {
	r := detection.NewQuestionProfileRegistry()
	r.Register(ClaudeQuestionProfile{})
	r.Register(binaries.NewOpenCodeQuestionProfile())
	return r
}
