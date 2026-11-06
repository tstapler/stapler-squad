package session

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// TabbedQuestion is one question of a multi-question AskUserQuestion.
type TabbedQuestion struct {
	Text        string
	Header      string
	Labels      []string
	MultiSelect bool
}

var (
	// ErrTabbedShapeUnmeasured: only the shape captured in Spike S1c is typed;
	// any other combination of questions would be an unverified key sequence.
	ErrTabbedShapeUnmeasured = errors.New("this question shape has no measured key sequence")
	// ErrTabbedAnswer: the answers do not fit the questions.
	ErrTabbedAnswer = errors.New("answers do not match the questions")
)

// MeasuredTabbedShape reports whether qs is a shape with a recorded key
// sequence: exactly two questions, a single-select then a multi-select (the
// Spike S1c fixtures). Multi-select first, two single-selects and 3-4 questions
// were not captured, so they stay read-only.
func MeasuredTabbedShape(qs []TabbedQuestion) bool {
	return len(qs) == 2 && !qs[0].MultiSelect && qs[1].MultiSelect &&
		len(qs[0].Labels) > 0 && len(qs[1].Labels) > 0
}

// PlanTabbedAnswer returns the key steps that answer qs with picks (one list of
// labels per question; exactly one for a single-select, at least one for a
// multi-select). Every step carries the frame expected right before its key,
// computed from the answers, never read from the pane. The sequence is the one
// recorded in claude_auq_twoq_keys.txt: Enter on the focused single-select
// option (it advances), Down/Enter to toggle each multi-select row, Down to the
// Submit row, Enter to open the review screen, Enter on `Submit answers`.
func PlanTabbedAnswer(qs []TabbedQuestion, picks [][]string) ([]ReplyStep, error) {
	if !MeasuredTabbedShape(qs) {
		return nil, ErrTabbedShapeUnmeasured
	}
	one, many, err := validTabbedPicks(qs, picks)
	if err != nil {
		return nil, err
	}
	p := &tabbedPlanner{qs: qs, answered: []bool{false, false}}
	p.planSingle(one)
	p.planMulti(many)
	p.add(KeyEnter, p.question(1, p.focus))
	p.add(KeyEnter, p.review(picks))
	return p.steps, nil
}

func validTabbedPicks(qs []TabbedQuestion, picks [][]string) (single int, multi []bool, err error) {
	if len(picks) != len(qs) || len(picks[0]) != 1 || len(picks[1]) == 0 {
		return 0, nil, fmt.Errorf("%w: need one choice for question 1 and at least one for question 2", ErrTabbedAnswer)
	}
	single = slices.Index(qs[0].Labels, picks[0][0])
	if single < 0 {
		return 0, nil, fmt.Errorf("%w: %q is not an option of question 1", ErrTabbedAnswer, picks[0][0])
	}
	multi = make([]bool, len(qs[1].Labels))
	for _, l := range picks[1] {
		i := slices.Index(qs[1].Labels, l)
		if i < 0 || multi[i] {
			return 0, nil, fmt.Errorf("%w: %q is not a distinct option of question 2", ErrTabbedAnswer, l)
		}
		multi[i] = true
	}
	return single, multi, nil
}

type tabbedPlanner struct {
	qs       []TabbedQuestion
	answered []bool
	checked  []bool
	focus    int
	steps    []ReplyStep
}

func (p *tabbedPlanner) add(keys string, e TabbedExpect) {
	p.steps = append(p.steps, ReplyStep{Keys: keys, Expect: e.Matches})
}

func (p *tabbedPlanner) question(i, focus int) TabbedExpect {
	q := p.qs[i]
	return TabbedExpect{
		Headers: headersOf(p.qs), Answered: slices.Clone(p.answered),
		Question: q.Text, Labels: q.Labels, Multi: q.MultiSelect,
		Checked: slices.Clone(p.checked), Focus: focus,
	}
}

func (p *tabbedPlanner) planSingle(want int) {
	for p.focus < want {
		p.add(KeyDown, p.question(0, p.focus))
		p.focus++
	}
	p.add(KeyEnter, p.question(0, p.focus))
	p.answered[0] = true
	p.focus = 0
	p.checked = make([]bool, len(p.qs[1].Labels))
}

func (p *tabbedPlanner) planMulti(want []bool) {
	submit := len(want) + 1
	for j, sel := range want {
		if !sel {
			continue
		}
		for p.focus < j {
			p.add(KeyDown, p.question(1, p.focus))
			p.focus++
		}
		p.add(KeyEnter, p.question(1, p.focus))
		p.checked[j] = true
		p.answered[1] = true // the tab flips to answered at the first toggle (fixture AB_toggled)
	}
	for p.focus < submit {
		p.add(KeyDown, p.question(1, p.focus))
		p.focus++
	}
}

func (p *tabbedPlanner) review(picks [][]string) TabbedExpect {
	multi := make([]string, 0, len(picks[1]))
	for _, l := range p.qs[1].Labels {
		if slices.Contains(picks[1], l) {
			multi = append(multi, l)
		}
	}
	return TabbedExpect{
		Headers: headersOf(p.qs), Answered: []bool{true, true}, Review: true,
		Summary: []TabbedSummary{
			{Question: p.qs[0].Text, Answer: picks[0][0]},
			{Question: p.qs[1].Text, Answer: strings.Join(multi, ", ")},
		},
	}
}

func headersOf(qs []TabbedQuestion) []string {
	h := make([]string, len(qs))
	for i, q := range qs {
		h[i] = q.Header
	}
	return h
}

// TabbedStillOpen is the closed check for the sequence: the dialog is open
// while any recognised tabbed frame is on screen.
func TabbedStillOpen(capture string) bool {
	_, ok := ParseTabbedFrame(capture)
	return ok
}
