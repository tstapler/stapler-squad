package jira

import (
	"encoding/json"
	"strings"
)

type adfNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Attrs   json.RawMessage `json:"attrs"`
	Content []adfNode       `json:"content"`
}

// ADFToPlainText converts a JIRA Cloud Atlassian Document Format value to
// readable plaintext. A JSON string (legacy wiki-markup description) passes
// through unchanged; null/empty yields "". Node types outside the supported
// set render as "[unsupported content omitted: <type>]" rather than vanishing.
func ADFToPlainText(doc json.RawMessage) string {
	if len(doc) == 0 || string(doc) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(doc, &s) == nil {
		return s
	}
	var root adfNode
	if json.Unmarshal(doc, &root) != nil {
		return ""
	}
	var b strings.Builder
	renderADF(&b, root, "")
	return strings.TrimSpace(b.String())
}

func attrString(raw json.RawMessage, key string) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func inlineText(n adfNode) string {
	var b strings.Builder
	renderADF(&b, n, "")
	return strings.TrimSpace(b.String())
}

func renderChildren(b *strings.Builder, n adfNode, prefix string) {
	for _, c := range n.Content {
		renderADF(b, c, prefix)
	}
}

func renderADF(b *strings.Builder, n adfNode, prefix string) {
	switch n.Type {
	case "doc", "blockquote", "panel", "expand", "tableCell", "tableHeader", "listItem", "taskItem":
		renderChildren(b, n, prefix)
	case "text":
		b.WriteString(n.Text)
	case "hardBreak":
		b.WriteString("\n")
	case "paragraph", "heading":
		renderChildren(b, n, prefix)
		b.WriteString("\n")
	case "bulletList":
		for _, item := range n.Content {
			b.WriteString("- " + inlineText(item) + "\n")
		}
	case "orderedList":
		for i, item := range n.Content {
			b.WriteString(itoa(i+1) + ". " + inlineText(item) + "\n")
		}
	case "taskList":
		for _, item := range n.Content {
			box := "- [ ] "
			if attrString(item.Attrs, "state") == "DONE" {
				box = "- [x] "
			}
			b.WriteString(box + inlineText(item) + "\n")
		}
	case "codeBlock":
		b.WriteString("```\n")
		renderChildren(b, n, prefix)
		b.WriteString("\n```\n")
	case "table":
		for _, row := range n.Content {
			cells := make([]string, 0, len(row.Content))
			for _, cell := range row.Content {
				cells = append(cells, inlineText(cell))
			}
			b.WriteString(strings.Join(cells, " | ") + "\n")
		}
	case "mention":
		b.WriteString(attrString(n.Attrs, "text"))
	case "inlineCard", "blockCard":
		b.WriteString(attrString(n.Attrs, "url"))
		if n.Type == "blockCard" {
			b.WriteString("\n")
		}
	case "emoji":
		b.WriteString(attrString(n.Attrs, "text"))
	case "rule":
		b.WriteString("---\n")
	default:
		b.WriteString("[unsupported content omitted: " + n.Type + "]\n")
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

// PlainTextToADF wraps plaintext in a minimal ADF document (one paragraph per
// line) for use as an outbound comment body.
func PlainTextToADF(text string) map[string]any {
	var paras []any
	for _, line := range strings.Split(text, "\n") {
		p := map[string]any{"type": "paragraph"}
		if line != "" {
			p["content"] = []any{map[string]any{"type": "text", "text": line}}
		}
		paras = append(paras, p)
	}
	return map[string]any{"type": "doc", "version": 1, "content": paras}
}
