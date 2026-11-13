package pipeline

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// TestRenderTemplateAndValidatePipelineModeContent_should_AgreeOnEveryToken_When_TableDrivenOverAllPlaceholdersPlusOneBogusToken
// is table-driven over the 7 recognized placeholder names plus one synthetic
// unrecognized name (made_up_placeholder). For each: renderTemplate
// substitutes it (no literal {{name}} survives) if-and-only-if
// ValidatePipelineModeContent accepts a template using only that token. This
// is the behavioral cross-check that catches drift even if the two
// functions do NOT literally share a slice.
func TestRenderTemplateAndValidatePipelineModeContent_should_AgreeOnEveryToken_When_TableDrivenOverAllPlaceholdersPlusOneBogusToken(t *testing.T) {
	t.Parallel()
	tokens := make([]string, 0, len(session.RecognizedPlaceholders)+1)
	tokens = append(tokens, session.RecognizedPlaceholders...)
	tokens = append(tokens, "made_up_placeholder")

	item := &backlogdto.BacklogItemData{ID: "item-1", Title: "Title", Description: "Desc", RepoPath: "/repo"}
	placeholders := itemPlaceholders(item)
	placeholders["criteria_index"] = "1"
	placeholders["criteria_count"] = "3"
	placeholders["criteria_text"] = "criterion text"

	for _, token := range tokens {
		t.Run(token, func(t *testing.T) {
			t.Parallel()
			tmpl := "{{" + token + "}}"

			rendered := renderTemplate(tmpl, placeholders)
			substituted := !strings.Contains(rendered, "{{"+token+"}}")

			err := session.ValidatePipelineModeContent(session.PipelineModeContentFields{
				TriagePromptTemplate: tmpl,
			})
			accepted := err == nil

			assert.Equal(t, substituted, accepted,
				"token %q: renderTemplate substituted=%v but ValidatePipelineModeContent accepted=%v (err=%v)",
				token, substituted, accepted, err)
		})
	}
}
