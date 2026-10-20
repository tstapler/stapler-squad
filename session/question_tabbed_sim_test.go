package session

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// A renderer and key-driven simulator for the Spike S1c dialog (Q1 single:
// Redis/Memcached/None, Q2 multi: A/B/C). TestTabbedRenderer_* pins the
// renderer byte-for-byte to the real captures, so the frames the sequence
// tests step through are the recorded layout, not an invention.

const tabRule = "────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────"

const (
	footerPlain  = "Enter to select · Tab/Arrow keys to navigate · Esc to cancel"
	footerSubmit = "Enter to select · Tab/Arrow keys to navigate · ctrl+g to edit in Nvim · Esc to cancel"
)

// fromTabBar returns the fixture text starting at its tab bar.
func fromTabBar(s string) string {
	i := strings.Index(s, "←")
	if i < 0 {
		return s
	}
	return s[i:]
}

var (
	q1Rows = []struct{ label, desc string }{
		{"Redis", "In-memory data store with rich data types and optional persistence"},
		{"Memcached", "Simple, multi-threaded in-memory key-value cache"},
		{"None", "Do not use a cache"},
	}
	q2Labels = []string{"A", "B", "C"}
)

func mark(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// renderQ1 draws the single-select frame with the cursor on option focus.
func renderQ1(focus int) string {
	var b strings.Builder
	b.WriteString("←  ☐ Cache  ☐ Letters  ✔ Submit  →\n\nWhich cache?\n\n")
	for i, r := range q1Rows {
		b.WriteString(mark(i == focus, "❯ ", "  ") + string(rune('1'+i)) + ". " + r.label + "\n     " + r.desc + "\n")
	}
	b.WriteString("  4. Type something.\n" + tabRule + "\n  5. Chat about this\n\n" + footerPlain)
	return b.String()
}

// renderQ2 draws the multi-select frame: focus 0-2 options, 3 Type something, 4 Submit.
func renderQ2(q1Answered bool, checked [3]bool, focus int) string {
	var b strings.Builder
	answered := checked[0] || checked[1] || checked[2]
	b.WriteString("←  " + mark(q1Answered, "☒", "☐") + " Cache  " + mark(answered, "☒", "☐") + " Letters  ✔ Submit  →\n\nWhich letters?\n\n")
	for i, l := range q2Labels {
		b.WriteString(mark(i == focus, "❯ ", "  ") + string(rune('1'+i)) + ". " + mark(checked[i], "[✔]", "[ ]") + " " + l + "\n         Option " + l + "\n")
	}
	b.WriteString(mark(focus == 3, "❯ ", "  ") + "4. [ ] Type something\n" + mark(focus == 4, "❯    ", "     ") + "Submit\n")
	b.WriteString(tabRule + "\n  5. Chat about this\n\n" + mark(focus == 4, footerSubmit, footerPlain))
	return b.String()
}

func renderReview(a1, a2 string) string {
	return "←  ☒ Cache  ☒ Letters  ✔ Submit  →\n\nReview your answers\n\n ● Which cache?\n   → " + a1 +
		"\n ● Which letters?\n   → " + a2 + "\n\nReady to submit your answers?\n\n❯ 1. Submit answers\n  2. Cancel"
}

const fixturePrefix = "\n ▐▛███▛█   Claude Code v2.1.303\n\n❯ Use the AskUserQuestion tool.\n" + tabRule + "\n"

func TestTabbedRenderer_ShouldReproduceTheRecordedCaptures(t *testing.T) {
	for name, want := range map[string]string{
		"claude_auq_twoq_step1_single_select.txt":              renderQ1(0),
		"claude_auq_twoq_step2_multiselect_initial.txt":        renderQ2(true, [3]bool{}, 0),
		"claude_auq_twoq_step2_multiselect_AB_toggled.txt":     renderQ2(true, [3]bool{true, true, false}, 1),
		"claude_auq_twoq_step2_multiselect_submit_focused.txt": renderQ2(true, [3]bool{true, true, false}, 4),
		"claude_auq_twoq_review_submit.txt":                    renderReview("Redis", "A, B"),
	} {
		got := strings.TrimRight(want, "\n")
		fixture := strings.TrimRight(fromTabBar(loadDialogFixture(t, name)), "\n")
		require.Equal(t, fixture, got, name)
	}
}

// tabbedSim is a pane that behaves like the recorded dialog under Enter and Down.
type tabbedSim struct {
	mu      sync.Mutex
	tab     int // 0 Q1, 1 Q2, 2 review, 3 closed
	q1Focus int
	q1Pick  int
	checked [3]bool
	q2Focus int
	keys    []string
	// hooks for fault tests
	failAt   int            // 1-based write index that fails; 0 = never
	failN    int            // bytes reported with the failure
	failErr  error          // the failure
	override map[int]string // after write index k, show this frame instead
	frozen   bool           // keys are accepted but the screen never changes
	owner    string
}

func (s *tabbedSim) LeaseOwnerUUID() string { return s.owner }

func (s *tabbedSim) SendKeysN(keys string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failAt > 0 && len(s.keys)+1 == s.failAt {
		if s.failN > 0 {
			s.keys = append(s.keys, keys)
		}
		return s.failN, s.failErr
	}
	s.keys = append(s.keys, keys)
	if s.frozen {
		return len(keys), nil
	}
	switch keys {
	case KeyDown:
		s.down()
	case KeyEnter:
		s.enter()
	}
	return len(keys), nil
}

func (s *tabbedSim) down() {
	switch s.tab {
	case 0:
		s.q1Focus = min(s.q1Focus+1, 2)
	case 1:
		s.q2Focus = min(s.q2Focus+1, 4)
	}
}

func (s *tabbedSim) enter() {
	switch s.tab {
	case 0:
		s.q1Pick, s.tab = s.q1Focus, 1
	case 1:
		switch {
		case s.q2Focus < 3:
			s.checked[s.q2Focus] = !s.checked[s.q2Focus]
		case s.q2Focus == 4:
			s.tab = 2
		}
	case 2:
		s.tab = 3
	}
}

func (s *tabbedSim) CapturePaneContent() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.override[len(s.keys)]; ok {
		return fixturePrefix + f, nil
	}
	switch s.tab {
	case 0:
		return fixturePrefix + renderQ1(s.q1Focus), nil
	case 1:
		return fixturePrefix + renderQ2(true, s.checked, s.q2Focus), nil
	case 2:
		var ans []string
		for i, c := range s.checked {
			if c {
				ans = append(ans, q2Labels[i])
			}
		}
		return fixturePrefix + renderReview(q1Rows[s.q1Pick].label, strings.Join(ans, ", ")), nil
	}
	return "\n\n" + tabRule + "\n❯ \n" + tabRule + "\n  ? for shortcuts\n", nil
}

func (s *tabbedSim) written() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.keys...)
}
