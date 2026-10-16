package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestADFToPlainText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"paragraph", `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Fix the login bug"}]}]}`, "Fix the login bug"},
		{"null", `null`, ""},
		{"legacy string", `"plain wiki text"`, "plain wiki text"},
		{"bullets", `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]}`, "- a\n- b"},
		{"codeBlock", `{"type":"doc","content":[{"type":"codeBlock","content":[{"type":"text","text":"panic: x"}]}]}`, "```\npanic: x\n```"},
		{"taskList", `{"type":"doc","content":[{"type":"taskList","content":[{"type":"taskItem","attrs":{"state":"DONE"},"content":[{"type":"text","text":"done"}]},{"type":"taskItem","attrs":{"state":"TODO"},"content":[{"type":"text","text":"todo"}]}]}]}`, "- [x] done\n- [ ] todo"},
		{"table", `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"k"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"v"}]}]}]}]}]}`, "k | v"},
		{"inlineCard", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"inlineCard","attrs":{"url":"https://x.atlassian.net/browse/A-1"}}]}]}`, "https://x.atlassian.net/browse/A-1"},
		{"mention", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"text":"@bob"}}]}]}`, "@bob"},
		{"unsupported", `{"type":"doc","content":[{"type":"mediaSingle"}]}`, "[unsupported content omitted: mediaSingle]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ADFToPlainText(json.RawMessage(c.in)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestPlainTextToADF_should_RoundTripThroughADFToPlainText(t *testing.T) {
	raw, _ := json.Marshal(PlainTextToADF("line one\nline two"))
	if got := ADFToPlainText(raw); !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Fatalf("got %q", got)
	}
}
