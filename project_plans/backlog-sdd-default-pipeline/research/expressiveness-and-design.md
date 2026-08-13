# Research: does the existing PipelineMode system express "run through SDD phases"?

Combines what would otherwise be 4 parallel research passes (stack/features/architecture/pitfalls)
into one document, since almost the entire stack/architecture surface is already fully documented in
`project_plans/backlog-configurable-pipeline/research/*.md` and `implementation/plan.md` — re-deriving
it would duplicate, not add, information. This document only covers what's genuinely new: whether the
already-shipped seam can express an SDD-flavored mode, and the concrete content design.

## 1. Expressiveness verdict: yes for triage/initial/commands, partially no for review

Traced every `PipelineEngine` render call site (`session/pipeline_engine.go`):

| Method | Placeholders available to a custom mode | Verdict |
|---|---|---|
| `SlashCommandSet` | item_id/title/description/repo_path + criteria_count, + criteria_index/text per-criterion | Sufficient — default's own content is this simple |
| `TriagePromptFor` | item_id/title/description/repo_path only — **`artifactAbsPath` param is silently dropped** for custom modes | Workable, but can't reference the platform's own artifact directory |
| `InitialPromptFor` | item_id/title/description/repo_path only | Sufficient — this is the prompt I control most; instructing the spawned session (which has full tool access) to invoke the SDD skills itself works within this budget |
| `ReviewPromptFor` / `InteractiveReviewPromptFor` | item_id/title/description/repo_path + criteria_count — **`diff`, `acSnapshot`, `verificationNotes`, `extras` are all silently dropped** for custom modes | Materially deficient for the headless (tool-less) path; workable for the interactive (tool-having) path only |

Both drops are real, pre-existing, and not sdd-specific — any custom `PipelineMode` today loses this
context. `recognizedPlaceholders` (`pipeline_engine.go`) has no `artifact_path`, `diff`, or
`acceptance_criteria_list` entries, and `ValidatePipelineModeContent` enforces this same allow-list at
write time, so a mode author cannot even ask for these by adding a `{{...}}` token — it would be
rejected as unrecognized.

**Decision: work around, don't fix.** Fixing this properly means extending `itemPlaceholders`/the
render call sites in the shared `pipeline_engine.go` seam, which has its own large test suite
(`pipeline_engine_test.go` implied by the Pattern Decisions table) and is consulted by every pipeline
mode, not just this one. That's a real, scoped follow-up (worth its own SDD pass), not something to
fold into "ship the sdd default." The workaround:
- **Triage**: the sdd `TriagePromptTemplate` doesn't need `artifactAbsPath` — SDD skills already have
  their own artifact home (`project_plans/<slug>/`, this exact repo's own convention, used by 90+
  existing directories), so the seeded content tells the session to pick its own slug and write there
  instead of trying to reconstruct the platform's external artifact path.
- **Review**: the sdd `ReviewPromptTemplate` is designed for the **interactive** review-gate session
  (`InteractiveReviewPromptFor` — "the review path most items actually go through" per
  `pipeline_engine.go`'s own doc comment), which has real tool access. Rather than depending on an
  embedded diff, it instructs the reviewer to gather the diff itself (`git diff` against the base
  branch) — which is *also* what `sdd:6-verify`'s own Step 1 already does, so this isn't a workaround
  bolted on top of SDD, it's exactly SDD's own verify process. A secondary paragraph gives a
  degraded-but-safe JSON-output fallback for the tool-less headless path (used by the narrower,
  manual-trigger-only `TriggerReReview` flow), explicit that it must mark criteria UNVERIFIABLE
  rather than guess when it cannot see a diff.

## 2. Does PipelineEngine need new phase-tracking state? No.

Confirmed by reading `BuildHeadlessTriagePrompt` (the *default* triage prompt): it already asks the
headless session to "run 4 subagents in parallel" and write `plan.md`/`validation.md` before its final
JSON-only output — i.e., the existing seam already proves prompt content alone can drive a
multi-step, tool-using, multi-file-writing sequence inside one headless call. The sdd mode's
distinction from default is *which* multi-step process runs (the actual phase-gated `sdd:2-research`
→ `sdd:3-plan` → `sdd:4-validate` skills, with their adversarial review and pre-mortem, instead of one
ad hoc inline instruction) and that `sdd:6-verify` runs before requesting review — both expressible as
prompt text instructing the spawned session (interactive or headless-with-tools) to invoke the Skill
tool / matching slash command itself. The session already has that tool access
(`headless.CallOptions{WorkDir: itemRepoPath}` in `backlog_service_triage.go` sets no
`AllowedTools`/`DisallowedTools` restriction for triage calls — full tool access, confirmed by
inspection). No new `PipelineEngine` method, no new DB column, no new control-flow state needed.

## 3. Content-authoring constraint: `ValidatePipelineModeContent`

`session/pipeline_mode_validation.go` rejects any content-template field containing a backtick, `` $( ``,
`;`, `|`, or `&&` (defense-in-depth against a field ever being shell-interpreted, per the parent
project's NFR on structural integrity). This ruled out markdown inline-code formatting
(`` `/backlog/done-0` ``) and pipe-table syntax in every seeded template — plain-text command
references and prose lists only. Confirmed by reading the validator directly, not by trial and error,
since a validation failure at seed time would silently no-op the whole feature (seed logs a warning
and continues per the non-fatal-boot NFR) rather than loudly failing in a way anyone would notice —
the seed *test* asserts every field passes `ValidatePipelineModeContent`, closing that gap.

## 4. Where "which mode is default for new items" belongs

Traced how `BacklogItemForm.tsx` submits `pipelineMode`: it is **always sent**, even when `""`
(`pipelineMode: data.pipelineMode ?? ""` in `useBacklogService.ts`'s create call), because the form's
`pipelineMode` state itself defaults to `""` via `useState(initialValues?.pipelineMode ?? "")`. This
means a backend-side "default when the proto field is `nil`" branch (`CreateBacklogItemRequest`'s
`pipeline_mode` is `optional string`, so a true Go `nil` pointer is distinguishable from an explicit
empty string) is **never exercised by the form itself** — only by callers that omit the field
entirely (the e2e debug seed handler, future scripts). The frontend pre-selection is therefore the
*primary* mechanism for the form path; the backend default is a secondary, defense-in-depth path for
everything else. Both are implemented, but they are not redundant — they cover disjoint call sites.
