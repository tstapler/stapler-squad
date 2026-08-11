package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ParseHeadlessTriageResult ────────────────────────────────────────────────

func TestParseHeadlessTriageResult_ValidJSON(t *testing.T) {
	raw := `{"summary":"Nice summary","suggestions":[{"text":"do X","rationale":"why"}],"tasks":[{"text":"task 1","estimate":"2h","category":"backend"}]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "Nice summary", result.Summary)
	require.Len(t, result.Suggestions, 1)
	assert.Equal(t, "do X", result.Suggestions[0].Text)
	require.Len(t, result.Tasks, 1)
	assert.Equal(t, "2h", result.Tasks[0].Estimate)
}

func TestParseHeadlessTriageResult_StripsMarkdownFences(t *testing.T) {
	raw := "```json\n{\"summary\":\"fenced\",\"suggestions\":[]}\n```"
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "fenced", result.Summary)
}

func TestParseHeadlessTriageResult_InvalidJSON(t *testing.T) {
	_, err := ParseHeadlessTriageResult("{not json}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParseHeadlessTriageResult")
}

func TestParseHeadlessTriageResult_CapsTasksAt12(t *testing.T) {
	tasks := make([]string, 0, 15)
	for i := range 15 {
		tasks = append(tasks, `{"text":"t`+string(rune('0'+i))+`","estimate":"1h","category":"backend"}`)
	}
	raw := `{"summary":"x","suggestions":[],"tasks":[` + strings.Join(tasks, ",") + `]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Len(t, result.Tasks, maxHeadlessTriageTasks)
}

func TestParseHeadlessTriageResult_EmptySuggestionsOK(t *testing.T) {
	raw := `{"summary":"minimal","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "minimal", result.Summary)
	assert.Empty(t, result.Tasks)
}

func TestParseHeadlessTriageResult_PreambleBeforeJSON(t *testing.T) {
	raw := "Here is my analysis of the backlog item.\nSome additional notes.\n" +
		`{"summary":"preamble ok","suggestions":[{"text":"s","rationale":"r"}]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "preamble ok", result.Summary)
}

func TestParseHeadlessTriageResult_PreambleBeforeFencedJSON(t *testing.T) {
	// Most common real-world case: Claude outputs text, then a fenced block.
	raw := "Triage complete. Here is the result:\n\n" +
		"```json\n" +
		`{"summary":"fenced ok","suggestions":[]}` +
		"\n```"
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "fenced ok", result.Summary)
}

func TestParseHeadlessTriageResult_NoJSON(t *testing.T) {
	_, err := ParseHeadlessTriageResult("No JSON here at all.")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParseHeadlessTriageResult")
}

// TestParseHeadlessTriageResult_PrematureCompletionPlaceholder is a regression test
// for the live incident tracked in docs/tasks/backlog-feature-improvement.md's
// 2026-07-30 entry (backlog item 04089969, log line
// staplersquad-2026-07-29T22-55-04.915.log.gz:32397). Running the "sdd" pipeline
// mode's triage prompt, the headless call ended its turn with a status update
// describing a still-running background subagent instead of the final JSON block —
// the raw output was exactly the string below (rawLen=164 in the original log).
// ParseHeadlessTriageResult must reject this the same as any other non-JSON output
// (asserted here) so TriggerTriage's caller can distinguish "no result yet" from "a
// result that happens to look empty" and the item becomes eligible for the
// orphaned_triage stuck-detector's automatic retry (session/backlog_lifecycle.go's
// reconcileOrphanedTriageItems, "shape 2" — see
// TestReconcileOrphanedTriageItems_should_flagImmediately_When_TriageSessionEndedWithoutTransition
// for the detector-side half of this regression).
func TestParseHeadlessTriageResult_PrematureCompletionPlaceholder(t *testing.T) {
	raw := "Planning subagent is running in the background to write `plan.md`. I'll wait for its completion before dispatching the architecture/adversarial/UX review subagents."
	_, err := ParseHeadlessTriageResult(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParseHeadlessTriageResult")
	assert.Contains(t, err.Error(), "no JSON object found")
}

// TestParseHeadlessTriageResult_StrayBraceInPreamble is a regression test for the
// audit finding in project_plans/backlog-cross-platform-audit/gaps-and-risks.md #5:
// the old first-`{`/last-`}` scan spans across an unrelated illustrative brace in
// prose and the real JSON object, producing an unparseable concatenated blob. Real
// models sometimes describe their output format before emitting it, e.g. "the
// result looks like {"example":"schema"}" — this must not break parsing of the real
// object that follows.
func TestParseHeadlessTriageResult_StrayBraceInPreamble(t *testing.T) {
	raw := `I've finished the triage. For reference, a sample response looks like {"example":"schema"}.

` + `{"summary":"the real summary","suggestions":[{"text":"s","rationale":"r"}],"tasks":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "the real summary", result.Summary)
}

// TestParseHeadlessTriageResult_MultipleStrayBracesPickLast verifies the parser
// prefers the LAST balanced JSON object when multiple valid-looking candidates are
// present, matching the prompt's instruction to emit the real result last.
// TestParseHeadlessTriageResult_PriorResultEchoedThenRevised verifies that when a
// refine prompt causes the model to quote the full prior HeadlessTriageResult JSON
// (e.g. while explaining what it changed) before emitting the revised result, the
// revised (last) object is the one returned — not the echoed prior one.
func TestParseHeadlessTriageResult_PriorResultEchoedThenRevised(t *testing.T) {
	raw := `Here was the prior result for reference:
{"title":"old-title","summary":"old summary","suggestions":[],"tasks":[{"text":"old task","estimate":"1h","category":"backend"}]}

Incorporating the feedback, here is the revised result:
{"title":"new-title","summary":"new summary","suggestions":[],"tasks":[{"text":"new task","estimate":"2h","category":"frontend"}]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "new-title", result.Title)
	assert.Equal(t, "new summary", result.Summary)
	require.Len(t, result.Tasks, 1)
	assert.Equal(t, "new task", result.Tasks[0].Text)
}

func TestParseHeadlessTriageResult_MultipleStrayBracesPickLast(t *testing.T) {
	raw := `First I considered {"summary":"decoy one"} but discarded it.
Then {"summary":"decoy two","suggestions":[]} also didn't fit.
Final answer: {"summary":"real one","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "real one", result.Summary)
}

// TestParseHeadlessTriageResult_BraceInsideStringLiteralIgnored verifies that a
// brace character embedded inside a JSON string value does not confuse the
// balanced-scan depth counter, even with a decoy object in the preamble whose own
// depth-counting would go wrong if string literals weren't tracked correctly: were
// the scanner to naively count every `{`/`}` regardless of string context, the
// decoy's un-nested brace plus the real object's embedded `{ braces }` would throw
// off depth tracking and either merge the two spans or truncate the real one early.
func TestParseHeadlessTriageResult_BraceInsideStringLiteralIgnored(t *testing.T) {
	raw := `Example: {"x":"y"}
Real: {"summary":"config looks like { nested }","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "config looks like { nested }", result.Summary)
}

// TestParseHeadlessTriageResult_EscapedQuoteInStringLiteral verifies that an
// escaped quote (`\"`) inside a JSON string value does not prematurely end
// string-literal tracking and expose the brace-depth counter to characters that
// are still logically inside the string.
func TestParseHeadlessTriageResult_EscapedQuoteInStringLiteral(t *testing.T) {
	raw := `{"summary":"She said \"the plan has a { in it\" during standup","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, `She said "the plan has a { in it" during standup`, result.Summary)
}

// TestParseHeadlessTriageResult_EscapedBackslashBeforeQuote verifies that a literal
// escaped backslash (`\\`) immediately followed by a closing quote is not
// misread as an escaped quote (`\"`) — the escape flag must reset after consuming
// the backslash, not be evaluated against the following quote character.
func TestParseHeadlessTriageResult_EscapedBackslashBeforeQuote(t *testing.T) {
	raw := `{"summary":"path is C:\\","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, `path is C:\`, result.Summary)
}

// TestParseHeadlessTriageResult_StrayClosingBraceBeforeJSON verifies parsing still
// succeeds when the preamble contains a stray, unmatched closing brace (not a
// complete decoy object) before the real JSON.
func TestParseHeadlessTriageResult_StrayClosingBraceBeforeJSON(t *testing.T) {
	raw := `Here's a stray closing brace: } — ignore that.
{"summary":"still parses","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "still parses", result.Summary)
}

// TestParseHeadlessTriageResult_StrayUnmatchedOpeningBrace is a regression test
// for a bug caught in code review: a hand-rolled brace-depth counter permanently
// "gets stuck" once it sees an opening brace that never closes — depth never
// returns to zero, so every well-formed object after it is silently swallowed and
// the parser wrongly reports "no JSON object found" even though a valid result
// follows. The fix decodes independently from each `{`, so one unmatched brace
// cannot poison the rest of the scan.
func TestParseHeadlessTriageResult_StrayUnmatchedOpeningBrace(t *testing.T) {
	raw := `Note: { this brace never closes, it's just a stray character.
{"summary":"still found me","suggestions":[]}`
	result, err := ParseHeadlessTriageResult(raw)
	require.NoError(t, err)
	assert.Equal(t, "still found me", result.Summary)
}

// ─── BuildHeadlessTriagePrompt ────────────────────────────────────────────────

func TestBuildHeadlessTriagePrompt_ContainsTitle(t *testing.T) {
	item := &BacklogItemData{Title: "My Feature", ID: "abc-123"}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp/artifacts")
	assert.Contains(t, prompt, "My Feature")
}

func TestBuildHeadlessTriagePrompt_ContainsArtifactPath(t *testing.T) {
	item := &BacklogItemData{Title: "Test", ID: "id-1"}
	prompt := BuildHeadlessTriagePrompt(item, "/repo/docs/tasks/test")
	assert.Contains(t, prompt, "/repo/docs/tasks/test")
}

func TestBuildHeadlessTriagePrompt_InstructsJSONOutput(t *testing.T) {
	item := &BacklogItemData{Title: "Test", ID: "id-1"}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp/art")
	// Headless mode must instruct JSON output, not call an MCP tool.
	assert.Contains(t, prompt, "output ONLY a JSON object",
		"headless prompt must instruct JSON output mode")
}

func TestBuildHeadlessTriagePrompt_IncludesAcceptanceCriteria(t *testing.T) {
	// ParseAcCriteria expects JSON-encoded criteria.
	acJSON := AcCriteriaJSON(`[{"index":1,"text":"User can log in","status":"pending"},{"index":2,"text":"User can log out","status":"pending"}]`)
	item := &BacklogItemData{
		Title:              "AC Test",
		ID:                 "id-2",
		AcceptanceCriteria: acJSON,
	}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp")
	assert.Contains(t, prompt, "User can log in")
}

func TestBuildHeadlessTriagePrompt_NoAcSection_WhenEmpty(t *testing.T) {
	item := &BacklogItemData{Title: "No AC", ID: "id-3"}
	prompt := BuildHeadlessTriagePrompt(item, "/tmp")
	assert.NotContains(t, prompt, "Acceptance Criteria")
}

// ─── BuildHeadlessRetriagePrompt ──────────────────────────────────────────────

func TestBuildHeadlessRetriagePrompt_ContainsPriorResultAndFeedback(t *testing.T) {
	item := &BacklogItemData{Title: "My Feature", ID: "abc-123"}
	prior := HeadlessTriageResult{
		Iteration: 1,
		Summary:   "Original plan summary.",
		Tasks:     []TriageTask{{Text: "Do the thing", Estimate: "2h", Category: "backend"}},
	}
	prompt := BuildHeadlessRetriagePrompt(item, "/tmp/artifacts", prior, "This missed the mobile case entirely.")

	assert.Contains(t, prompt, "Original plan summary.")
	assert.Contains(t, prompt, "Do the thing")
	assert.Contains(t, prompt, "This missed the mobile case entirely.")
	assert.Contains(t, prompt, "output ONLY a JSON object")
}

func TestBuildHeadlessRetriagePrompt_ReferencesExistingArtifacts(t *testing.T) {
	item := &BacklogItemData{Title: "My Feature", ID: "abc-123"}
	prompt := BuildHeadlessRetriagePrompt(item, "/tmp/artifacts", HeadlessTriageResult{}, "feedback text")
	assert.Contains(t, prompt, "/tmp/artifacts/plan.md")
	assert.Contains(t, prompt, "do not start from scratch")
}
