package session

import (
	"fmt"
	"strings"
)

// Canonical wording shared by every work-session prompt: the default builders
// (backlog_context.go, backlog_commands.go) and the seeded sdd pipeline mode
// (pipeline_mode_seed.go). Edit a rule here once instead of in each copy.
//
// Two constraints shape the text. Seeded templates are stored in the DB and
// ValidatePipelineModeContent rejects backticks, "$(", ";", "|" and "&&", so
// sections never use the last four (TestPromptSections_should_PassSeedValidation
// enforces it). Backticks are written as codeMark and stripped for seeds.
// Per-item values use the {{placeholders}} renderTemplate already substitutes.
const codeMark = "¦"

// forDefault renders a section for the Go-built prompts (code spans in backticks).
func forDefault(section string) string { return strings.ReplaceAll(section, codeMark, "`") }

// forSeed renders a section for a seeded template (no backticks).
func forSeed(section string) string { return strings.ReplaceAll(section, codeMark, "") }

// renderDefault renders a section for the Go-built prompts, substituting placeholder values.
func renderDefault(section string, values map[string]string) string {
	return renderTemplate(forDefault(section), values)
}

const secWorkDiscipline = `For a bug, first write one sentence "this fails because X" and confirm X with a reproducing command or test before you change code. ` +
	`Before ¦/backlog/done-N¦, run the command or test that proves criterion N and put the exact command and its one-line result in the note. ` +
	`If you only read the code, say so. ` +
	`If a test fails, re-run it on a clean main first: when it fails there too it is pre-existing, so root-cause it and fix it in its own commit if small, otherwise record it with post_backlog_update. ` +
	`To click through a change by hand, follow CLAUDE.md's "Manual/interactive testing without touching the live deployed instance" ` +
	`(a distinct PORT, STAPLER_SQUAD_INSTANCE and STAPLER_SQUAD_TMUX_SOCKET every time) and kill that instance yourself when done. ` +
	`Unclosed instances have exhausted this machine's memory.`

const secBlockerRouting = `If a criterion cannot be met (ambiguous, contradictory, or it depends on a failure that will not reproduce or on an unrelated flaky test) ` +
	`or you need a decision only the owner can make, call the request_owner_action MCP tool ` +
	`(item_id, a question that includes your evidence, allowed_responses, and waive_criterion_index=N when the owner can waive criterion N). ` +
	`It notifies the owner and wakes you with the answer, so do not poll. Keep working on the other criteria meanwhile. ` +
	`For an external blocker (missing credential, unmerged dependency, CI outage) run ¦/backlog/block¦. ` +
	`¦/backlog/review¦ is rejected while any criterion is unmet, so never use it to report a blocker.`

const secContextLost = `If your context is compacted or you lose track of your task, re-read ¦.backlog-context.md¦ or run ¦/backlog/status¦ immediately before continuing.`

const secCommandsFail = `If the ¦/backlog/*¦ commands fail or the MCP server is unavailable, continue your work using the criteria listed in ¦.backlog-context.md¦ and record completed criteria in your commit messages.`

const secNeverEnd = `NEVER end your session without calling ¦/backlog/review¦ — this is how the task is closed properly.`

const secAwaitVerdict = `end your turn and stay idle in this session (do not exit). Do NOT poll, and do NOT use ScheduleWakeup or /loop to wait: every wake re-reads your whole context. ` +
	`The app sends you a message with the verdict as soon as it is recorded.`

const secOnPass = `PASS: run ¦/backlog/ship¦ yourself to open the pull request (it drives ¦/github:pr-ship¦, which can rebase, resolve merge conflicts, and react to failing CI checks). ` +
	`Do not stop here. Shipping the PR is part of this task, not a separate step someone else does.`

// secOnFail takes MaxSameSessionReviewAttempts.
const secOnFail = `FAIL or PARTIAL: fix the noted gaps in this same session and run ¦/backlog/review¦ again. ` +
	`Its request_review call reports which attempt you are on out of %d allowed in THIS session. The count is tracked server-side, so trust what it reports rather than counting your own calls. ` +
	`Once it says you have hit the cap, STOP looping: run ¦/backlog/ship¦ anyway to open a PR so a human can pick up the review directly, rather than retrying ¦/backlog/review¦.`

func onFailSection() string { return fmt.Sprintf(secOnFail, MaxSameSessionReviewAttempts) }

const secDoneNote = `Call report_progress with item_id={{item_id}}, criteria_index={{criteria_index}}, status=pass. ` +
	`Before calling this, run the command or test that proves the criterion. Include the optional note parameter with that exact command and its one-line result ` +
	`(for example "go test ./session -run TestX: ok"). If you only read the code, say so, and do not report pass for anything you have not executed. ` +
	`The note goes to this item's permanent audit trail as context for a reviewer, who weighs the proving commands and output you pass in request_review's verification_notes as the evidence.`

const secFailNote = `Call report_progress with item_id={{item_id}}, criteria_index={{criteria_index}}, status=fail. ` +
	`Include the optional note parameter with a 1-2 sentence explanation of specifically what is blocking this criterion. ` +
	`It is appended to this item's permanent audit trail and is what a reviewer or the next session reads to understand why this criterion is not done yet. ` +
	`If the criterion cannot be met as written, also follow the owner-action guidance in /backlog/help, because /backlog/review is rejected while any criterion is unmet.`

const secShipIntro = `You are ready to ship your work as a pull request — either because ¦/backlog/review¦ just returned PASS, ` +
	`or because review has looped without reaching a PASS and it is time to hand the work to a human instead of retrying indefinitely.

Before shipping, run ¦/backlog/status¦. Every criterion must be done or waived. If one is neither, use request_owner_action rather than shipping around it.`

const secShipSteps = `Steps:
1. Create the pull request: run ¦/github:pr-ship¦. It drives the PR through local CI, code review, remote CI, and merge-conflict resolution. It will stop short of actually merging, because the final merge is left to the human reviewer.
2. Once ¦/github:pr-ship¦ reports all gates green: if this work has NOT already received a PASS verdict (you are shipping because review looped without converging, not because it passed), request the automated review with the PR number included by running ¦/backlog/review¦ with a 2-3 sentence summary of what was built and the PR number. If review already returned PASS before you got here, skip this step. Running it again will fail because the item is no longer in_progress, and there is nothing left for it to check.
3. Report the PR back onto this backlog item — REQUIRED, do not skip: call the report_pr_created MCP tool with item_id={{item_id}}, pr_url=<the PR URL printed by ¦/github:pr-ship¦ or ¦gh pr create¦>, pr_number=<the PR number>, and summary=<2-3 sentences: what changed and why>. You created each PR yourself, so nothing else will ever report it back to the item record. Skipping this step leaves the item stuck in review with a real PR that is invisible to the reviewer and the operator.

If the repository has no GitHub remote, run ¦gh pr create¦ manually. Do NOT use ¦--fill¦, which just concatenates commit messages with no test plan. Write ¦--title¦ in Conventional Commits format and a ¦--body¦ with a Summary section (why this change was made, from the backlog item), a What Changed bullet list, and a Test plan checklist of concrete verification steps. Then run ¦/backlog/review¦, then step 3 above to report the PR.`

const helpOwnerActionLine = `request_owner_action (MCP tool) — ask the owner to decide, or to waive a criterion you cannot meet (waive_criterion_index=N)`

const secTriageCriteriaCheck = `For each acceptance criterion, name the command or test that would prove it. ` +
	`If none exists, or the criterion depends on a failure that will not reproduce or on an unrelated flaky test, ` +
	`add a suggestion with rationale="question" instead of keeping an unattainable criterion.`

// triageOutputContract is the JSON-only result every triage prompt ends with. The parser
// (ParseHeadlessTriageResult) is the same for all of them.
// estimate is a cost class, not hours: agents do the work, so wall-clock time is meaningless.
func triageOutputContract() string {
	return triageContract(triageExampleWithACs, []string{triageTitleLine, triageSummaryLine, triageACLine, triageSuggestionsLine, triageTasksLine, triageNoSubmitLine})
}

// retriageOutputContract is triageOutputContract for the feedback-driven retriage, which
// revises tasks and suggestions but not the acceptance criteria.
func retriageOutputContract() string {
	return triageContract(triageExampleNoACs, []string{triageTitleLine, triageSummaryLine, triageSuggestionsLine, triageTasksLine, triageNoSubmitLine})
}

func triageContract(example string, bullets []string) string {
	return strings.Join(append([]string{example}, bullets...), "\n") + "\n"
}

const (
	triageExampleWithACs  = `{"title":"fix-short-kebab-name","summary":"2-3 sentence summary","acceptance_criteria":[{"index":0,"text":"Clear, testable criterion","status":"pending"}],"suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"task description","estimate":"M","category":"backend"}]}`
	triageExampleNoACs    = `{"title":"fix-short-kebab-name","summary":"2-3 sentence summary","suggestions":[{"text":"...","rationale":"..."}],"tasks":[{"text":"task description","estimate":"M","category":"backend"}]}`
	triageTitleLine       = `- title: short kebab-case session name (3-5 words, imperative verb first, e.g. "fix-session-rename" or "add-pr-status-badge")`
	triageSummaryLine     = `- summary: 2-3 sentence executive summary`
	triageACLine          = `- acceptance_criteria: full list of testable acceptance criteria (replace any existing ones). Each has index (0-based), text (one clear testable statement), status ("pending"). Merge with existing criteria: keep unchanged ones, add new ones, update clarified ones.`
	triageSuggestionsLine = `- suggestions: additional open questions or improvement ideas beyond the criteria (questions use rationale="question")`
	triageTasksLine       = `- tasks: implementation task breakdown (max 12). estimate is a cost class, never hours or days: S (one agent turn), M (one agent wave), L (several waves)`
	triageNoSubmitLine    = `- Do NOT call submit_triage_result. Do NOT write any source code.`
)

const secReviewFailureClass = `For each criterion that is not PASS, set failure_class so the app routes it correctly: ` +
	`NOT_DONE (default) when the worker has more to do; ` +
	`CANNOT_VERIFY when you lack access to the evidence; ` +
	`UNRELATED_FAILURE when the proof fails only because of a test or code the change did not touch; ` +
	`ENVIRONMENTAL when tooling, network, or CI outside the change is broken; ` +
	`WRONG_BRANCH when the cited commits are not on the reviewed branch; ` +
	`UNATTAINABLE when the criterion cannot be met as written. ` +
	`Anything except NOT_DONE goes to the owner instead of another rework round, so use it only when more work in this change cannot fix the gap. ` +
	`When you judge the owner should waive the criterion, also set proposed_waiver=true and put the reason in evidence.`
