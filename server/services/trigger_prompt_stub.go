package services

import (
	"bytes"
	"fmt"
	"text/template"
)

// renderTriggerPromptStub renders a Workflow's PromptTemplate against the parsed
// webhook payload using stdlib text/template.
//
// TODO(Phase 3): this is a deliberate, narrowly-scoped stand-in for
// workflows.RenderTriggerPrompt (project_plans/webhook-triggers/implementation/plan.md
// Task 3.1.1a / Task 3.2.1b), which does not exist yet in this branch. The real
// implementation additionally wraps the rendered output in the same
// inert-data-block framing session.BuildSessionInitialPrompt uses (prompt-injection
// defense — see plan.md's Pattern Decisions table) and applies
// sanitizeField/truncateField to untrusted payload values. This stub does neither: it
// is only responsible for proving the HTTP receiver, HMAC verification, matching, and
// dedup wiring are correct (Epic 2.2/2.3's actual scope). Replace call sites of this
// function with workflows.RenderTriggerPrompt once Task 3.2.1b lands — do not extend
// this stub with injection-defense logic, that duplication is exactly what the
// cross-phase note in this task's brief warns against.
func renderTriggerPromptStub(tmplStr string, payload map[string]interface{}) (string, error) {
	if tmplStr == "" {
		return "", fmt.Errorf("prompt_template is empty")
	}

	tmpl, err := template.New("trigger-prompt-stub").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse prompt_template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, payload); err != nil {
		return "", fmt.Errorf("render prompt_template: %w", err)
	}

	return buf.String(), nil
}
