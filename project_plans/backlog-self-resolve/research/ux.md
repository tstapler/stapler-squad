# UX Research — backlog-self-resolve (Agent 5)

Scope: this feature is two MCP tools (`request_review` generalization, new
`report_duplicate`) called only by AI agent sessions — there is no human-facing
UI in the acceptance criteria. "UX" here is the tool description text and
error/success message wording, which is the literal interface the calling LLM
agent reads and acts on. Web accessibility/WCAG research is out of scope per
the task framing (machine-to-machine interface, not rendered UI).

## 1. Does the frontend render `verification_notes` (or would it need to render `duplicate_ref`/`reason`)?

**No.** `verification_notes` (the closest existing precedent to
`duplicate_ref`/`reason` — freeform evidence text captured from a work-role
tool call) is **never rendered in any React component**. Confirmed by:

- Repo-wide grep for `verification_notes`/`verificationNotes` under
  `web-app/src/` returns zero matches.
- The field's only consumer is the *review-gate LLM prompt*, not a human UI:
  `session/storage_backlog.go:55` defines it on `ItemSessionData`
  (`VerificationNotes string // Freeform verification evidence reported via request_review`),
  `session/storage_backlog.go:395` persists it
  (`UpdateItemSessionVerificationNotes`), and `session/review_gate.go:315`
  feeds it straight into `reviewPromptFor(...)` — the prompt handed to the
  *reviewing agent session*, not to a human operator.
- `server/mcp/tools_backlog.go:420-427` (`requestReview`) confirms this is a
  best-effort, agent-to-agent handoff: "Persist verification evidence on the
  work ItemSession so the review gate can surface it in the reviewer's
  prompt."

**Implication for `report_duplicate`:** persisting `duplicate_ref`/`reason`
the same way (backend-only, feeding the next reviewer's prompt, no new React
component) is consistent with existing precedent, not a regression relative
to how `verification_notes` already works. It would leave the evidence
invisible to a human operator watching the board — but that gap already
exists today for `verification_notes`, so `report_duplicate` following suit
does not introduce a new inconsistency. If FR3/FR5's "routes to review"
requirement is read strictly (evidence must reach *some* reviewer, human or
AI), backend-only persistence satisfies it exactly like `verification_notes`
does. Flagging this for the plan to make an explicit call rather than
defaulting silently: **no UI changes are required to be spec-compliant**, but
if a human operator ever needs to see *why* an item was flagged duplicate
without opening the review-gate's prompt/log, that's a follow-up gap
identical to the pre-existing `verification_notes` one — not something this
feature should be asked to fix as a side effect.

Files checked: `web-app/src/components/backlog/BacklogItemCard.tsx`,
`BacklogItemDetail.tsx`, `detail/LifecycleSummary.tsx`,
`BacklogItemPanel.tsx` — none reference `verification_notes`,
`duplicate_ref`, or `pr_pending`-style evidence fields beyond status badges.

## 2. House style for MCP tool descriptions (`server/mcp/tools_backlog.go`)

All backlog tools are registered in `registerBacklogTools`
(`server/mcp/tools_backlog.go:920-1090`) via
`mcpgo.NewTool(name).WithDescription(...)`. The established voice/format:

- **First sentence = one-line summary of effect.** E.g. `report_pr_created`:
  "Report a pull request YOU created (e.g. via `/backlog:ship` or a manual
  `gh pr create`) back onto this backlog item." `request_review`: "Signal
  that implementation is complete and the item is ready for review."
- **Second sentence = `Role: <work|review|triage> only.`** — every tool
  states its allowed caller role explicitly, immediately after the summary.
  E.g. `"Role: work only — do not call from triage or review sessions."`
  (`report_progress`), `"Role: review only."` (`submit_review_verdict`).
- **Then preconditions/sequencing** — when to call it relative to other
  tools: `"Call after all acceptance criteria are marked pass."`,
  `"Call this LAST — after all research/*.md, plan.md, and validation.md
  files are written."`
- **Then consequence/what happens on success** — state transitions are named
  explicitly by their string status values: `"Transitions the item to
  'review' status..."`, `"On success, the item transitions from review to
  pr_pending."`
- **Verification/anti-hallucination framing is a recurring theme.**
  `request_review`'s description spends 3 sentences telling the agent the
  reviewer "CANNOT see command output or UI behavior you observed," demands
  citations for "already implemented" claims, and explicitly calls out that
  unsupported claims are "likely to be marked UNVERIFIABLE." This is the
  house pattern for steering agent *behavior*, not just documenting the API
  shape — tool descriptions double as prompt-engineering surfaces.
- **Idempotency is stated explicitly when it applies.**
  `report_pr_created`: "Calling this again with the same PR after it already
  succeeded is safe (no-op)." This directly matters for FR10 — an agent
  deciding whether to retry needs to know retrying won't double-apply an
  effect.
- **Per-field `Description()` calls** repeat/extend guidance at the parameter
  level, often with a concrete example format, e.g. `verification_notes`'s
  description gives two full example strings
  (`"ran \`go test ./session/...\` -> ok (41 tests)"`).

`report_duplicate`'s description should match: one-line summary → `Role:
work only.` → preconditions (verified against GitHub) → success
consequence (routes to review) → retry/idempotency guidance for FR10 (see
§3) → accurate "next review pass" framing for FR5.

## 3. Retry vs. do-not-retry error message patterns

The `errResult(code, message, remediation string)` helper
(`server/mcp/tools_discovery.go:73-77`) has two channels for guidance: the
`message` itself, and a separate `remediation` field (both land in the JSON
error envelope's `Error.Message` / `Error.Remediation`). House style mixes
both, but the **pattern that most directly maps to FR10** is
`report_pr_created`'s GitHub-verification call
(`server/mcp/tools_backlog.go:707-710`):

```go
matched, verifyErr := h.verifyPR(ctx, ref.Owner, ref.Repo, prNumber, branch)
if verifyErr != nil {
    return errResult(ErrInternalError, fmt.Sprintf("could not verify PR #%d against GitHub — retry: %v", prNumber, verifyErr), ""), nil
}
```

This is the exact wording precedent for FR10 (`report_duplicate` verifying
`duplicate_ref` against GitHub can fail the same way — network error, GitHub
API hiccup, rate limit). The word **"retry"** is embedded directly in the
`message` string, not the `remediation` field, when the failure is a
transient infra call (GitHub API) rather than a user input problem.

Contrast with the **non-retryable** case, one branch below
(`tools_backlog.go:711-715`), which is `ErrInvalidArgument`, not
`ErrInternalError`, and uses "refusing"/"double-check" language instead of
retry language:

```go
if !matched {
    return errResult(ErrInvalidArgument, fmt.Sprintf(
        "PR #%d does not match this item's branch %q on GitHub — refusing to record it. Double-check the PR number/URL.",
        prNumber, branch), ""), nil
}
```

The house convention, generalized:

| Error code | Meaning to the agent | Wording pattern |
|---|---|---|
| `INTERNAL_ERROR` | Transient/infra failure, safe to retry | `"could not <verb> — retry: %v"` (retry word inline in message) |
| `INVALID_ARGUMENT` | Caller's input is wrong, retrying with the same args will fail identically | `"refusing to ... Double-check ..."` / states exactly what to fix |
| Idempotent success no-op | Already-applied call is safe to repeat | Stated in the **tool description**, not an error at all — e.g. `report_pr_created`'s "Calling this again... is safe (no-op)." |

Other `remediation`-field retry/backoff examples elsewhere in the file
(non-backlog tools, same codebase voice) for cross-reference:
`tools_terminal.go:278` (`"RATE_LIMITED"` → `"Wait 1 second before
retrying"`), `tools_terminal.go:305` (`"PTY_WRITE_TIMEOUT"` →
`"The session may be blocked. Use send_control with key=C to interrupt"`),
`tools_github.go:140` (`ErrRateLimitExceeded` → `"Wait before creating
another session."`). These show `remediation` is used for *mechanical*
retry instructions (wait N seconds, use tool X first) while inline
message-text "retry" is used for *judgment* calls (transient failure, just
call again).

**Recommendation for FR10:** `report_duplicate`'s description should state
explicitly, in the house's `request_review`-style verification-framing
paragraph, something like: "If verifying `duplicate_ref` against GitHub
fails with INTERNAL_ERROR, this is a transient failure — retry the call
with the same arguments. Do not fall back to marking the item non-duplicate
just because verification failed once." And the actual `errResult` call
site verifying against GitHub should follow the `report_pr_created`
precedent exactly: `fmt.Sprintf("could not verify %s against GitHub —
retry: %v", duplicateRef, verifyErr)` with `ErrInternalError`.

## 4. `BacklogStatusEvent` audit-trail rendering — does the status transition itself reach a human, even though `verification_notes` doesn't?

Yes, and this is a distinct channel from `verification_notes` (§1) that §1 didn't
cover. `BacklogStatusEvent`/`recordStatusEvent` (FR7's required mechanism for the
status transition) **is** rendered today:
[`web-app/src/components/backlog/detail/WorkflowHistorySection.tsx`](../../../web-app/src/components/backlog/detail/WorkflowHistorySection.tsx)
renders every event as a `fromStatus → toStatus` row with timestamp, `triggeredBy`,
and an optional plain-text `ev.note` (lines 44-58: `{ev.note && <span
className={styles.workflowEventNote}>{ev.note}</span>}`). This is the one channel
where a human operator watching the board *will* see this feature's effect,
without opening logs or a review-gate prompt — it renders unconditionally
whenever `report_duplicate` (or the generalized `request_review`) writes a
`BacklogStatusEvent` with `TriggeredBy="agent"` per FR7.

`ev.note` is rendered as opaque plain text (no markdown, no structured
key/value display) — same rendering path every other status-event note already
uses. So no new formatting/structure is needed for `report_duplicate`'s status
event note beyond what any other transition note already provides: a short,
human-legible sentence. **Recommendation for the plan phase:** make sure the
status-event `note` argument passed to `recordStatusEvent` for this transition
is actually populated with something like `"duplicate of <duplicate_ref>: <reason>"`
— not left empty while only `ItemSession.VerificationNotes` gets the detail.
`VerificationNotes` (§1) is the only channel that reaches the *next reviewer
agent's prompt*; the status-event `note` is the only channel that reaches a
*human* without extra digging. Populating both from the same call is cheap and
closes the "why is this review-via-duplicate rather than review-via-normal-work"
confusion named in the JTBD framing — with zero new UI code, since
`WorkflowHistorySection.tsx` already renders whatever note is given.

## 5. Existing stuck-item notification UI — does it already cover FR10's "eventually surfaces" claim?

Yes, and independently of the MCP tool description work in §2/§3. A mature,
pre-existing stuck-item detection + notification system covers this without any
new plumbing:

- Backend: `domain.StuckReason*` constants and `MarkStuck`/detector sweeps in
  [`session/backlog_lifecycle.go`](../../../session/backlog_lifecycle.go) —
  e.g. the `pr_pending_no_pr` detector (~line 2527) already flags `pr_pending`
  items with no PR reference, structurally the same shape an item is left in if
  `report_duplicate`'s GitHub verification fails and the caller doesn't retry.
- Frontend:
  [`web-app/src/components/backlog-stuck/StuckItemsSection.tsx`](../../../web-app/src/components/backlog-stuck/StuckItemsSection.tsx)
  renders a grouped, filterable "Stuck Backlog Items" list on `/unfinished`,
  plus [`StuckNavBadge.tsx`](../../../web-app/src/components/backlog-stuck/StuckNavBadge.tsx)
  (nav badge) and `stuckReason.ts` (per-reason label mapping) — all pre-existing,
  unaffected by this feature.

This resolves the open research question in `requirements.md` ("Where/how the
existing stuck-item notification path surfaces `pr_pending` items today, to
confirm FR10's 'eventually surfaces' claim is achievable without new plumbing"):
it is achievable via the existing `pr_pending`-status detector family, no new
detector or UI component required. The remaining open item is a **backend**
research question, not UX: confirming the specific detector's staleness
threshold/trigger window also fires for "stuck via failed duplicate
verification" the same way it fires for "stuck via no PR reference" — worth
flagging to whichever research track owns `session/backlog_lifecycle.go` detector
logic, not something this UX pass can resolve on its own.

## FR5 — accurate "next review pass" messaging

No existing tool message currently states "the current reviewer" vs "the
next review pass" distinction — this is new territory, not a precedent to
match. But the pattern for stating item-state-dependent success text
already exists in `requestReview`
(`server/mcp/tools_backlog.go:431-446`): it branches on `targetStatus` and
returns a *different* success string depending on which state the item
landed in (`SkipReviewGate` → done directly, vs normal → moved to review
and reviewer notified). `report_duplicate`/generalized `request_review`
should follow this same branching-success-message pattern: check whether a
review session is already active for the item before composing the success
string, and if so, explicitly say evidence "will be picked up on the next
review pass" rather than implying the current reviewer will see it live —
mirroring how `targetStatus == session.BacklogStatusDone` gets its own
distinct message rather than reusing the generic one.
