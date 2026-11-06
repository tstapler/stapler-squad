package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var twoQuestions = []TabbedQuestion{
	{Text: "Which cache?", Header: "Cache", Labels: []string{"Redis", "Memcached", "None"}},
	{Text: "Which letters?", Header: "Letters", Labels: []string{"A", "B", "C"}, MultiSelect: true},
}

// keyNames maps the tmux key names of claude_auq_twoq_keys.txt to the bytes written.
var keyNames = map[string]string{"Enter": KeyEnter, "Down": KeyDown}

func recordedKeys(t *testing.T) []string {
	t.Helper()
	var keys []string
	for _, l := range strings.Split(loadDialogFixture(t, "claude_auq_twoq_keys.txt"), "\n") {
		l = strings.TrimSpace(strings.SplitN(l, "#", 2)[0])
		if l == "" {
			continue
		}
		k, ok := keyNames[l]
		require.True(t, ok, "unknown key name %q in the fixture", l)
		keys = append(keys, k)
	}
	return keys
}

func TestParseTabbedFrame_ShouldParseTheRecordedCaptures(t *testing.T) {
	cases := []struct {
		fixture  string
		answered []bool
		review   bool
		question string
		rows     int
		focus    int
		checked  []bool
	}{
		{"claude_auq_twoq_step1_single_select.txt", []bool{false, false}, false, "Which cache?", 5, 0, nil},
		{"claude_auq_twoq_step2_multiselect_initial.txt", []bool{true, false}, false, "Which letters?", 6, 0, []bool{false, false, false}},
		{"claude_auq_twoq_step2_multiselect_AB_toggled.txt", []bool{true, true}, false, "Which letters?", 6, 1, []bool{true, true, false}},
		{"claude_auq_twoq_step2_multiselect_submit_focused.txt", []bool{true, true}, false, "Which letters?", 6, 4, []bool{true, true, false}},
		{"claude_auq_twoq_review_submit.txt", []bool{true, true}, true, "", 2, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			f, ok := ParseTabbedFrame(loadDialogFixture(t, c.fixture))
			require.True(t, ok)
			require.Len(t, f.Tabs, 2)
			assert.Equal(t, c.answered, []bool{f.Tabs[0].Answered, f.Tabs[1].Answered})
			assert.Equal(t, []string{"Cache", "Letters"}, []string{f.Tabs[0].Header, f.Tabs[1].Header})
			assert.Equal(t, c.review, f.Review)
			assert.Equal(t, c.question, f.Question)
			require.Len(t, f.Rows, c.rows)
			focus := -1
			var checked []bool
			for i, r := range f.Rows {
				if r.Focused {
					focus = i
				}
				if r.Boxed && i < 3 {
					checked = append(checked, r.Checked)
				}
			}
			assert.Equal(t, c.focus, focus)
			assert.Equal(t, c.checked, checked)
		})
	}
}

func TestParseTabbedFrame_ShouldReadTheReviewSummary(t *testing.T) {
	f, ok := ParseTabbedFrame(loadDialogFixture(t, "claude_auq_twoq_review_submit.txt"))
	require.True(t, ok)
	assert.Equal(t, []TabbedSummary{{"Which cache?", "Redis"}, {"Which letters?", "A, B"}}, f.Summary)
}

func TestDialogMatch_ShouldStillRejectTheTabbedLayout(t *testing.T) {
	// The tab bar replaces `☐ <header>`, so the single-select matcher fails closed and
	// the tabbed matcher is the only path for these frames.
	q := QuestionDialog{Question: "Which cache?", Header: "Cache", Labels: []string{"Redis", "Memcached", "None"}}
	assert.False(t, DialogMatch(loadDialogFixture(t, "claude_auq_twoq_step1_single_select.txt"), q))
}

func TestTabbedExpect_ShouldRejectForgedAndDriftedFrames(t *testing.T) {
	initial := loadDialogFixture(t, "claude_auq_twoq_step2_multiselect_initial.txt")
	want := TabbedExpect{
		Headers: []string{"Cache", "Letters"}, Answered: []bool{true, false},
		Question: "Which letters?", Labels: []string{"A", "B", "C"}, Multi: true, Checked: []bool{false, false, false}, Focus: 0,
	}
	require.True(t, want.Matches(initial), "the recorded frame matches its own expectation")

	composer := "\n" + tabRule + "\n❯ \n" + tabRule + "\n  ? for shortcuts\n"
	for name, capture := range map[string]string{
		"row after the footer":           initial + "\n  1. Allow once\n",
		"composer after the footer":      initial + composer,
		"scrollback copy under composer": strings.TrimRight(initial, "\n") + "\n" + composer,
		"toggled checkbox":               renderPrefixed(renderQ2(true, [3]bool{true}, 0)),
		"focus moved":                    renderPrefixed(renderQ2(true, [3]bool{}, 1)),
		"question one unanswered":        renderPrefixed(renderQ2(false, [3]bool{}, 0)),
		"other question text":            strings.ReplaceAll(initial, "Which letters?", "Which colors?"),
		"extra option row":               strings.Replace(initial, "  4. [ ] Type something", "  4. [ ] D\n         Option D\n  5. [ ] Type something", 1),
		"no tab bar":                     strings.Replace(initial, "←", "x", 1),
	} {
		assert.False(t, want.Matches(capture), name)
	}
}

func renderPrefixed(frame string) string { return fixturePrefix + frame }

func TestPlanTabbedAnswer_ShouldMatchTheRecordedKeySequence_When_AnswersAreRedisAndAB(t *testing.T) {
	steps, err := PlanTabbedAnswer(twoQuestions, [][]string{{"Redis"}, {"A", "B"}})
	require.NoError(t, err)
	got := make([]string, 0, len(steps))
	for _, s := range steps {
		got = append(got, s.Keys)
	}
	assert.Equal(t, recordedKeys(t), got)
}

func TestPlanTabbedAnswer_ShouldWalkToTheChosenRows_When_AnswersAreMemcachedAndAC(t *testing.T) {
	steps, err := PlanTabbedAnswer(twoQuestions, [][]string{{"None"}, {"A", "C"}})
	require.NoError(t, err)
	got := make([]string, 0, len(steps))
	for _, s := range steps {
		got = append(got, s.Keys)
	}
	// Q1: Down Down Enter. Q2: Enter(A) Down Down Enter(C) Down Down Enter(Submit) Enter(Submit answers).
	want := []string{KeyDown, KeyDown, KeyEnter, KeyEnter, KeyDown, KeyDown, KeyEnter, KeyDown, KeyDown, KeyEnter, KeyEnter}
	assert.Equal(t, want, got)
}

func TestPlanTabbedAnswer_ShouldRefuseUnmeasuredShapesAndBadAnswers(t *testing.T) {
	multiFirst := []TabbedQuestion{twoQuestions[1], twoQuestions[0]}
	_, err := PlanTabbedAnswer(multiFirst, [][]string{{"A"}, {"Redis"}})
	assert.ErrorIs(t, err, ErrTabbedShapeUnmeasured)
	_, err = PlanTabbedAnswer(twoQuestions[:1], [][]string{{"Redis"}})
	assert.ErrorIs(t, err, ErrTabbedShapeUnmeasured)
	three := append(append([]TabbedQuestion{}, twoQuestions...), twoQuestions[0])
	_, err = PlanTabbedAnswer(three, [][]string{{"Redis"}, {"A"}, {"Redis"}})
	assert.ErrorIs(t, err, ErrTabbedShapeUnmeasured)

	for name, picks := range map[string][][]string{
		"unknown single":   {{"Valkey"}, {"A"}},
		"two for single":   {{"Redis", "None"}, {"A"}},
		"empty multi":      {{"Redis"}, {}},
		"unknown multi":    {{"Redis"}, {"Z"}},
		"duplicate multi":  {{"Redis"}, {"A", "A"}},
		"one list only":    {{"Redis"}},
		"free text unsent": {{"Redis"}, {"Type something"}},
	} {
		_, err := PlanTabbedAnswer(twoQuestions, picks)
		assert.ErrorIs(t, err, ErrTabbedAnswer, name)
	}
}
