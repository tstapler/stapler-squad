package session

// question_tabbed.go matches the multi-question AskUserQuestion layout of
// Claude Code 2.1.303 (Spike S1c fixtures claude_auq_twoq_*.txt): a tab bar
// `←  ☐ Cache  ☒ Letters  ✔ Submit  →` replaces the bare `☐ <header>` line that
// DialogMatch needs, so DialogMatch fails closed on these frames. Every
// recognised frame is parsed into TabbedFrame; a layout it does not know is no match.

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	tabSubmitName      = "Submit"
	tabReviewTitle     = "Review your answers"
	tabReviewReady     = "Ready to submit your answers?"
	tabReviewSubmitRow = "Submit answers"
	tabReviewCancelRow = "Cancel"
	tabAnsweredGlyph   = "☒"
	tabOpenGlyph       = "☐"
	tabSubmitGlyph     = "✔"
	tabChecked         = "[✔]"
	tabUnchecked       = "[ ]"
	tabTypeSingle      = "Type something."
	tabTypeMulti       = "Type something"
)

var (
	tabBarLine   = regexp.MustCompile(`^← (.*) →$`)
	tabGlyphs    = regexp.MustCompile(`[☐☒✔] `)
	tabBoxRow    = regexp.MustCompile(`^(\[[^\]]\]) (.*)$`)
	tabRuleChars = regexp.MustCompile(`^─{10,}$`)
)

// TabbedTab is one question tab.
type TabbedTab struct {
	Header   string
	Answered bool
}

// TabbedRow is one selectable row of a frame.
type TabbedRow struct {
	Index   int // printed number; 0 for the unnumbered Submit row
	Label   string
	Boxed   bool // multi-select row with a checkbox
	Checked bool
	Focused bool
	Submit  bool // the unnumbered Submit row of a multi-select question
}

// TabbedSummary is one `● question / → answer` pair of the review screen.
type TabbedSummary struct{ Question, Answer string }

// TabbedFrame is one parsed frame of the tabbed multi-question dialog.
type TabbedFrame struct {
	Tabs     []TabbedTab
	Review   bool
	Question string // question frames only
	Rows     []TabbedRow
	Summary  []TabbedSummary // review frames only
}

type tabbedLine struct {
	text    string
	focused bool
}

func tabbedLines(capture string) []tabbedLine {
	raw := strings.Split(capture, "\n")
	out := make([]tabbedLine, len(raw))
	for i, l := range raw {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ansiEscape.ReplaceAllString(l, "")), dialogGutter))
		focused := strings.HasPrefix(l, string(dialogSelectionGlyph))
		if focused {
			l = strings.TrimPrefix(l, string(dialogSelectionGlyph))
		}
		out[i] = tabbedLine{text: strings.Join(strings.Fields(l), " "), focused: focused}
	}
	return out
}

// ParseTabbedFrame parses capture. It is bottom-anchored: a question frame must
// end with the `Enter to select` footer and a review frame with its `Cancel`
// row, so text an agent printed after (or above) the live dialog is no match.
func ParseTabbedFrame(capture string) (TabbedFrame, bool) {
	lines := tabbedLines(capture)
	bar := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if tabBarLine.MatchString(lines[i].text) {
			bar = i
			break
		}
	}
	if bar < 0 {
		return TabbedFrame{}, false
	}
	tabs, ok := parseTabBar(lines[bar].text)
	if !ok {
		return TabbedFrame{}, false
	}
	region := lines[bar+1:]
	for len(region) > 0 && region[len(region)-1].text == "" {
		region = region[:len(region)-1]
	}
	if len(region) == 0 {
		return TabbedFrame{}, false
	}
	frame := TabbedFrame{Tabs: tabs}
	if strings.HasPrefix(region[len(region)-1].text, dialogFooterPrefix) {
		return parseTabbedQuestion(frame, region[:len(region)-1])
	}
	return parseTabbedReview(frame, region)
}

// parseTabBar reads tab names and answered marks. The trailing `Submit` tab
// is required and carries no state of its own.
func parseTabBar(line string) ([]TabbedTab, bool) {
	inner := tabBarLine.FindStringSubmatch(line)[1] + " "
	locs := tabGlyphs.FindAllStringIndex(inner, -1)
	var tabs []TabbedTab
	for i, loc := range locs {
		end := len(inner)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		glyph, name := strings.TrimSpace(inner[loc[0]:loc[1]]), strings.TrimSpace(inner[loc[1]:end])
		if name == "" {
			return nil, false
		}
		if i == len(locs)-1 {
			return tabs, name == tabSubmitName && glyph == tabSubmitGlyph && len(tabs) > 0
		}
		if glyph != tabAnsweredGlyph && glyph != tabOpenGlyph {
			return nil, false
		}
		tabs = append(tabs, TabbedTab{Header: name, Answered: glyph == tabAnsweredGlyph})
	}
	return nil, false
}

func parseTabbedQuestion(frame TabbedFrame, region []tabbedLine) (TabbedFrame, bool) {
	var question []string
	sinceRow := 0 // unnumbered lines since the last numbered row
	for _, l := range region {
		switch {
		case l.text == "" || tabRuleChars.MatchString(l.text):
			continue
		case numberedRow.MatchString(l.text):
			row, ok := parseTabbedRow(l)
			if !ok {
				return TabbedFrame{}, false
			}
			frame.Rows = append(frame.Rows, row)
			sinceRow = 0
		case len(frame.Rows) == 0:
			question = append(question, l.text)
		case l.text == tabSubmitName && sinceRow == 0 && lastRowIsType(frame.Rows):
			frame.Rows = append(frame.Rows, TabbedRow{Label: tabSubmitName, Focused: l.focused, Submit: true})
		default:
			sinceRow++ // an option's description
		}
	}
	frame.Question = strings.Join(question, " ")
	return frame, frame.Question != "" && len(frame.Rows) > 0 && oneFocus(frame.Rows)
}

func lastRowIsType(rows []TabbedRow) bool {
	l := rows[len(rows)-1].Label
	return l == tabTypeMulti && rows[len(rows)-1].Boxed
}

func parseTabbedRow(l tabbedLine) (TabbedRow, bool) {
	m := numberedRow.FindStringSubmatch(l.text)
	idx, err := strconv.Atoi(m[1])
	if err != nil {
		return TabbedRow{}, false
	}
	row := TabbedRow{Index: idx, Label: m[2], Focused: l.focused}
	if b := tabBoxRow.FindStringSubmatch(m[2]); b != nil {
		switch b[1] {
		case tabChecked:
			row.Checked = true
		case tabUnchecked:
		default:
			return TabbedRow{}, false
		}
		row.Boxed, row.Label = true, b[2]
	}
	return row, true
}

func oneFocus(rows []TabbedRow) bool {
	n := 0
	for _, r := range rows {
		if r.Focused {
			n++
		}
	}
	return n == 1
}

// parseTabbedReview reads `Review your answers`, the `● q` / `→ a` pairs, the
// ready line and the two rows; the capture must end with `2. Cancel`.
func parseTabbedReview(frame TabbedFrame, region []tabbedLine) (TabbedFrame, bool) {
	for len(region) > 0 && region[0].text == "" {
		region = region[1:]
	}
	if len(region) == 0 || region[0].text != tabReviewTitle {
		return TabbedFrame{}, false
	}
	var q, a []string
	flush := func() {
		if len(q) > 0 {
			frame.Summary = append(frame.Summary, TabbedSummary{Question: strings.Join(q, " "), Answer: strings.Join(a, " ")})
		}
		q, a = nil, nil
	}
	state := ""
	for _, l := range region[1:] {
		switch {
		case l.text == "":
		case strings.HasPrefix(l.text, "● "):
			flush()
			q, state = append(q[:0], strings.TrimPrefix(l.text, "● ")), "q"
		case strings.HasPrefix(l.text, "→ ") && state == "q":
			a, state = append(a[:0], strings.TrimPrefix(l.text, "→ ")), "a"
		case l.text == tabReviewReady:
			flush()
			state = "rows"
		case state == "rows" && numberedRow.MatchString(l.text):
			row, ok := parseTabbedRow(l)
			if !ok {
				return TabbedFrame{}, false
			}
			frame.Rows = append(frame.Rows, row)
		case state == "q":
			q = append(q, l.text)
		case state == "a":
			a = append(a, l.text)
		default:
			return TabbedFrame{}, false
		}
	}
	frame.Review = true
	rows := frame.Rows
	ok := len(rows) == 2 && rows[0].Index == 1 && rows[0].Label == tabReviewSubmitRow &&
		rows[1].Index == 2 && rows[1].Label == tabReviewCancelRow && region[len(region)-1].text == "2. "+tabReviewCancelRow
	return frame, ok && oneFocus(rows)
}

// TabbedExpect is the frame a step must see before its key is written. It is
// derived from the answers being typed, never from the capture.
type TabbedExpect struct {
	Headers  []string
	Answered []bool
	// Review expects the review screen with Summary and focus on `Submit answers`.
	Review  bool
	Summary []TabbedSummary
	// Question frames:
	Question string
	Labels   []string
	Multi    bool
	Checked  []bool // multi only, per label
	// Focus indexes Labels, then `Type something`, then (multi) `Submit`.
	Focus int
}

// Matches reports whether capture is exactly this frame.
func (e TabbedExpect) Matches(capture string) bool {
	f, ok := ParseTabbedFrame(capture)
	if !ok || f.Review != e.Review || !e.tabsMatch(f.Tabs) {
		return false
	}
	if e.Review {
		return e.reviewMatches(f)
	}
	return e.questionMatches(f)
}

func (e TabbedExpect) tabsMatch(tabs []TabbedTab) bool {
	if len(tabs) != len(e.Headers) || len(e.Answered) != len(e.Headers) {
		return false
	}
	for i, t := range tabs {
		if normalizeDialogText(t.Header) != normalizeDialogText(e.Headers[i]) || t.Answered != e.Answered[i] {
			return false
		}
	}
	return true
}

func (e TabbedExpect) reviewMatches(f TabbedFrame) bool {
	if len(f.Summary) != len(e.Summary) || !f.Rows[0].Focused {
		return false
	}
	for i, s := range f.Summary {
		if s.Question != normalizeDialogText(e.Summary[i].Question) || s.Answer != normalizeDialogText(e.Summary[i].Answer) {
			return false
		}
	}
	return true
}

func (e TabbedExpect) questionMatches(f TabbedFrame) bool {
	n := len(e.Labels)
	want := n + 2 // options, Type something, Chat about this
	if e.Multi {
		want++ // the Submit row
	}
	if f.Question != normalizeDialogText(e.Question) || len(f.Rows) != want || (e.Multi && len(e.Checked) != n) {
		return false
	}
	focus := -1
	for i, r := range f.Rows {
		if r.Focused {
			focus = i
		}
	}
	if focus != e.Focus {
		return false
	}
	return e.rowsMatch(f.Rows)
}

func (e TabbedExpect) rowsMatch(rows []TabbedRow) bool {
	n := len(e.Labels)
	for i, label := range e.Labels {
		r := rows[i]
		if r.Index != i+1 || r.Label != normalizeDialogText(label) || r.Boxed != e.Multi || (e.Multi && r.Checked != e.Checked[i]) {
			return false
		}
	}
	typ, typeLabel := rows[n], tabTypeSingle
	if e.Multi {
		typeLabel = tabTypeMulti
	}
	if typ.Index != n+1 || typ.Label != typeLabel || typ.Boxed != e.Multi || typ.Checked {
		return false
	}
	last := rows[len(rows)-1]
	if last.Index != n+2 || last.Label != dialogChatAboutRow || last.Boxed {
		return false
	}
	return !e.Multi || rows[n+1].Submit
}
