package services

import (
	"strings"
	"unicode/utf8"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// AskedQuestion is the parsed AskUserQuestion payload. Question, Header and
// Labels describe question 1 (what PendingQuestionStore matches on screen);
// Payload holds all questions when they fit the domain limits.
type AskedQuestion struct {
	Shape    QuestionShape
	Question string
	Header   string
	Labels   []string
	// Payload is zero when the questions do not fit domain.QuestionPayload's limits.
	Payload domain.QuestionPayload
	// Replyable is true only for a single-select shape; multi-select and
	// multi-question stay false until stepwise answering (Story 3.5).
	Replyable bool
	// Tabbed is true for the one multi-question shape with a recorded key
	// sequence (session.MeasuredTabbedShape) whose texts and labels are usable.
	Tabbed bool
}

// ParseAskUserQuestion classifies tool_input. The real shape is
// {"questions":[{question, header, options:[{label, description}], multiSelect}]}
// (spike S1, Claude Code 2.1.303); any other payload is multi or unknown and
// not replyable. An absent multiSelect is unknown, never single. All text is
// run through sanitizeNotificationText.
func ParseAskUserQuestion(toolInput map[string]interface{}) AskedQuestion {
	raw, ok := toolInput["questions"].([]interface{})
	if !ok || len(raw) == 0 {
		return AskedQuestion{Shape: QuestionShapeUnknown}
	}
	var (
		qs           []domain.Question
		multiPresent = true
		firstMulti   bool
	)
	for i, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			return AskedQuestion{Shape: QuestionShapeUnknown}
		}
		q, present := parseQuestionMap(m)
		if i == 0 {
			firstMulti = q.MultiSelect
		}
		multiPresent = multiPresent && present
		qs = append(qs, q)
	}
	first := qs[0]
	out := AskedQuestion{Shape: QuestionShapeUnknown, Question: first.Text, Header: first.Header}
	for _, o := range first.Options {
		out.Labels = append(out.Labels, o.Label)
	}
	if payload, err := domain.NewQuestionPayload(qs); err == nil {
		out.Payload = payload
	}
	switch {
	case len(raw) > 1:
		out.Shape = QuestionShapeMulti
	case !multiPresent:
	case firstMulti:
		out.Shape = QuestionShapeMulti
	case replyableLabels(out.Labels) && strings.TrimSpace(out.Question) != "" && strings.TrimSpace(out.Header) != "":
		out.Shape = QuestionShapeSingle
	}
	out.Replyable = out.Shape == QuestionShapeSingle
	out.Tabbed = out.Shape == QuestionShapeMulti && multiPresent && tabbedReplyable(out.Payload)
	return out
}

// tabbedReplyable reports a payload the stepwise driver can answer: the measured
// two-question shape, every question with a text and a header, every label usable.
func tabbedReplyable(p domain.QuestionPayload) bool {
	if p.IsZero() {
		return false
	}
	qs := tabbedQuestions(p)
	if !session.MeasuredTabbedShape(qs) {
		return false
	}
	for _, q := range qs {
		if strings.TrimSpace(q.Text) == "" || strings.TrimSpace(q.Header) == "" || !replyableLabels(q.Labels) {
			return false
		}
	}
	return true
}

// parseQuestionMap reads one question; the bool reports whether multiSelect was present.
func parseQuestionMap(m map[string]interface{}) (domain.Question, bool) {
	var q domain.Question
	text, _ := m["question"].(string)
	header, _ := m["header"].(string)
	q.Text, q.Header = sanitizeNotificationText(text), sanitizeNotificationText(header)
	opts, _ := m["options"].([]interface{})
	for _, o := range opts {
		if om, ok := o.(map[string]interface{}); ok {
			label, _ := om["label"].(string)
			desc, _ := om["description"].(string)
			q.Options = append(q.Options, domain.Option{
				Label: sanitizeNotificationText(label), Description: sanitizeNotificationText(desc),
			})
		}
	}
	multi, present := m["multiSelect"].(bool)
	q.MultiSelect = multi
	return q, present
}

func replyableLabels(labels []string) bool {
	if len(labels) < 1 || len(labels) > pendingQuestionMaxOptions {
		return false
	}
	for _, l := range labels {
		if strings.TrimSpace(l) == "" || utf8.RuneCountInString(l) > pendingQuestionMaxLabelRunes {
			return false
		}
	}
	return true
}
