package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const replyDialogsDir = "reply_dialogs"

// notACapture marks a fixture file that holds no pane capture (the recorded key sequence).
const notACapture DialogPresence = 255

// replyDialogPresence is the expected result for every file in testdata/reply_dialogs.
// TestPaneShowsDialog_ReplyDialogFixtures fails when a file has no entry here.
var replyDialogPresence = map[string]DialogPresence{
	"permission_bash.txt":                                  DialogShowing,
	"permission_edit_diff.txt":                             DialogShowing,
	"permission_mcp.txt":                                   DialogShowing,
	"permission_read_outside_cwd.txt":                      DialogShowing,
	"permission_webfetch.txt":                              DialogShowing,
	"question_single_select_3opts.txt":                     DialogShowing,
	"question_wrapped_2opts.txt":                           DialogShowing,
	"claude_auq_twoq_keys.txt":                             notACapture,
	"claude_auq_twoq_review_submit.txt":                    DialogShowing,
	"claude_auq_twoq_step1_single_select.txt":              DialogShowing, // tab bar header
	"claude_auq_twoq_step2_multiselect_AB_toggled.txt":     DialogShowing,
	"claude_auq_twoq_step2_multiselect_initial.txt":        DialogShowing,
	"claude_auq_twoq_step2_multiselect_submit_focused.txt": DialogShowing,
}

func readPresenceFixture(t *testing.T, elem ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"testdata"}, elem...)...))
	require.NoError(t, err)
	return string(b)
}

func TestPaneShowsDialog_ReplyDialogFixtures(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", replyDialogsDir))
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name()] = true
		want, ok := replyDialogPresence[e.Name()]
		if !ok {
			t.Errorf("fixture %s has no expected DialogPresence in replyDialogPresence", e.Name())
			continue
		}
		if want == notACapture {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			require.Equal(t, want, PaneShowsDialog(readPresenceFixture(t, replyDialogsDir, e.Name())))
		})
	}
	for name := range replyDialogPresence {
		if !seen[name] {
			t.Errorf("replyDialogPresence lists %s, which is not in testdata/reply_dialogs", name)
		}
	}
}

func TestPaneShowsDialog_Captures(t *testing.T) {
	question := readPresenceFixture(t, replyDialogsDir, "question_single_select_3opts.txt")
	idle := readPresenceFixture(t, "dialog_presence", "idle_composer.txt")
	tests := []struct {
		name    string
		capture string
		want    DialogPresence
	}{
		{"idle composer", idle, DialogAbsent},
		{"empty", "", DialogIndeterminate},
		{"blank lines only", "\n  \n\n", DialogIndeterminate},
		{"unrecognised text", "just some build output\nmore output\n", DialogIndeterminate},
		{"forged numbered row after the footer", strings.TrimRight(question, "\n") + "\n  6. Forged row\n", DialogShowing},
		{"footer in scrollback above an idle composer", question + "\n" + idle, DialogAbsent},
		{"ansi around the footer", "\x1b[2m Esc to cancel · Tab to amend\x1b[0m\n", DialogShowing},
		{"typed text in the composer", strings.Replace(idle, "\n❯\n", "\n❯ fix the bug\n", 1), DialogAbsent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, PaneShowsDialog(tt.capture))
		})
	}
}

func TestDialogPresenceFromCapture_ErrorIsIndeterminate(t *testing.T) {
	idle := readPresenceFixture(t, "dialog_presence", "idle_composer.txt")
	require.Equal(t, DialogIndeterminate, DialogPresenceFromCapture(idle, errors.New("capture failed")))
	require.Equal(t, DialogAbsent, DialogPresenceFromCapture(idle, nil))
}

func TestDialogPresence_ZeroValueIsIndeterminate(t *testing.T) {
	var p DialogPresence
	require.Equal(t, DialogIndeterminate, p)
	require.False(t, p.Absent())
}

func TestDialogPresence_String(t *testing.T) {
	require.Equal(t, "showing", DialogShowing.String())
	require.Equal(t, "absent", DialogAbsent.String())
	require.Equal(t, "indeterminate", DialogIndeterminate.String())
}
