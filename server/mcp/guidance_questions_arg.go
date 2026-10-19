package mcp

import (
	"fmt"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/tstapler/stapler-squad/session/domain"
)

// parseGuidanceQuestionsArg reads create_guidance_request's optional
// "questions" argument into a validated payload. An absent argument yields a
// zero payload and no error; limits come from domain.NewQuestionPayload.
func parseGuidanceQuestionsArg(args map[string]any) (domain.QuestionPayload, *mcpgo.CallToolResult) {
	var zero domain.QuestionPayload
	raw, present := args["questions"]
	if !present || raw == nil {
		return zero, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return zero, errResult(ErrInvalidArgument, "questions must be a non-empty array", "")
	}
	qs := make([]domain.Question, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return zero, errResult(ErrInvalidArgument, fmt.Sprintf("questions[%d] must be an object", i), "")
		}
		q, err := questionFromArg(m)
		if err != nil {
			return zero, errResult(ErrInvalidArgument, fmt.Sprintf("questions[%d]: %v", i, err), "")
		}
		qs = append(qs, q)
	}
	payload, err := domain.NewQuestionPayload(qs)
	if err != nil {
		return zero, errResult(ErrInvalidArgument, err.Error(), "Limits: 1-4 questions, 1-8 options each.")
	}
	return payload, nil
}

func questionFromArg(m map[string]any) (domain.Question, error) {
	text, _ := m["text"].(string)
	if text == "" {
		text, _ = m["question"].(string)
	}
	header, _ := m["header"].(string)
	multi, _ := m["multi_select"].(bool)
	rawOpts, _ := m["options"].([]any)
	opts := make([]domain.Option, 0, len(rawOpts))
	for j, o := range rawOpts {
		switch v := o.(type) {
		case string:
			opts = append(opts, domain.Option{Label: v})
		case map[string]any:
			label, _ := v["label"].(string)
			desc, _ := v["description"].(string)
			opts = append(opts, domain.Option{Label: label, Description: desc})
		default:
			return domain.Question{}, fmt.Errorf("options[%d] must be a string or {label, description}", j)
		}
	}
	return domain.Question{Text: text, Header: header, Options: opts, MultiSelect: multi}, nil
}
