# Pitfalls Research: Plan Approval UX

**Phase**: 2 — Research (Agent 4: Pitfalls)
**Scope**: Failure modes and risks for plan-approval/reject-with-reason UX and
in-app markdown-artifact rendering, grounded in this repo's actual code (not
generic advice).

---

## 1. Stale-plan race: regeneration vs. an already-open browser tab

**Confirmed in code**: `PlanArtifactsPath` is a server-side directory path stored on
the `BacklogItem` (`session/repository.go:394`). It gets **overwritten in place** —
not versioned — on each triage/plan run:

- `server/services/backlog_service_triage.go:2081`: a feedback-driven re-triage
  writes a new `PlanArtifactsPath` via `session.BacklogItemUpdate{PlanArtifactsPath: &pap}`.
- `server/services/backlog_service_lifecycle.go:598-602`: transitioning an item
  backward to `idea`/`refining` resets `PlanApproved=false` **and clears
  `PlanArtifactsPath` to `""`** in the same update.
- `session/backlog_review.go:715-722` (`readPlanFile`) always reads
  `filepath.Join(artifactsDir, "plan.md")` fresh from disk — there is no content
  hash, ETag, or version counter anywhere in this path.

**The risk this creates**: if a new RPC is added to fetch and render plan content
by `item_id` (as the requirements imply), a user who has the plan open in a
detail pane while a background triage/plan regeneration completes will be looking
at content whose backing file may have been rewritten or deleted out from under
them. Two concrete failure shapes:

1. **Stale content, live buttons**: the rendered `plan.md` in the pane is the old
   version, but "Approve"/"Reject" actions fire against the *current* server-side
   `PlanArtifactsPath`/`PlanApproved` state — the user approves content they
   never actually saw.
2. **Approve/reject racing a path swap**: `ApprovePlan` (`backlog_service_lifecycle.go:625-654`)
   only checks `item.PlanArtifactsPath == ""` and `os.Stat` at call time — if a
   concurrent re-triage clears/reassigns the path between the user clicking
   "Approve" and the RPC executing, the approval could land against a
   *different* plan than what was rendered, or fail with a precondition error
   the UI doesn't explain (`no plan artifacts found` / `path does not exist on
   disk`).

**Design against**: include a content fingerprint (e.g. the `PlanArtifactsPath`
string itself, a mtime, or a hash of the fetched content) in the render RPC
response and echo it back on `ApprovePlan`/`RejectPlan` as an optimistic-concurrency
token; reject with a clear "plan changed since you loaded it — refresh" error
rather than silently approving/rejecting the wrong content. At minimum, surface
the existing `FailedPrecondition` errors from `ApprovePlan` in the UI instead of
a generic failure toast.

---

## 2. ent schema regen for new rejection fields — `--feature sql/upsert` trap

**Confirmed in code**: `session/ent/generate.go` declares the only correct
generate command:

```go
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```

Existing plan fields in `session/ent/schema/backlog_item.go`:
```go
field.Bool("plan_approved")...
field.Time("plan_approved_at")...
field.String("plan_artifacts_path")...
```

Adding parallel fields (`plan_rejected`, `plan_rejected_at`, `plan_rejection_reason`,
or similar) requires re-running codegen. Per
`.claude/rules/ent-schema-generation.md`, running the plain
`go run entgo.io/ent/cmd/ent generate ./session/ent/schema` (omitting
`--feature sql/upsert` and `-mod=mod`) **compiles successfully but silently
breaks `UpsertRule`-style generated methods** — no compiler error, no test
failure until an upsert path is actually exercised. Given `ent_repository_backlog.go`
already has an `Upsert*`-shaped update path (`u.SetPlanArtifactsPath(...)` at
line 606 inside what looks like a builder pattern), any codegen invoked with the
wrong flags during this feature's implementation is a silent regression risk.
**Always run `go generate ./session/ent/` or copy the exact command from
`generate.go`.**

---

## 3. `tests/e2e/plan-gate.spec.ts` — brittle exact-attribute assertions

**Confirmed in code**: the existing spec (`tests/e2e/plan-gate.spec.ts`) asserts
exact strings on the spawn button:

```ts
await expect(spawnBtn).toHaveAttribute('aria-disabled', 'true');
await expect(spawnBtn).toHaveAttribute('title', 'Approve the plan or enable skip_planning to spawn a session');
```

This is a **hard-coded copy string**, not just a state check. Any UI restructuring
that changes the disabled-button copy (e.g. to mention "or reject the plan" once
reject-with-reason exists, or to reference a new "changes requested" state) will
break this test even though the underlying gating logic is unchanged. Two
implications for planning:

- If the `Must Have` UX work changes this button's tooltip/title text at all,
  **`plan-gate.spec.ts` must be updated in the same PR**, not left to fail in CI.
  This is not optional collateral — it is a direct, foreseeable break, not
  incidental debt.
- The spec covers only the `DequeueNextQueuedItems` / queued-item path
  (`StuckReasonPlanNotApproved` gate). If Success Criterion #2 ("consistent
  gate") extends gating to other spawn paths, **new gate assertions are needed
  for those paths** — the existing spec does not exercise a direct-spawn (non-queue)
  path at all, so "consistent gate" work could ship with zero e2e coverage of the
  parts that are actually new.
- Per `.claude/rules/e2e-test-conventions.md`, any new spec/assertions must use
  `data-testid`/ARIA locators only (already followed here — `backlog-action-spawn-session`,
  `backlog-action-approve-plan`) and must not introduce `waitForTimeout`.

---

## 4. Markdown rendering: XSS surface if raw HTML is enabled

**Confirmed in code**: `react-markdown` (`^10.1.0`) and `remark-gfm` are already
dependencies, and there is a **working precedent**:
`web-app/src/components/backlog/detail/DescriptionSection.tsx`:
```tsx
<ReactMarkdown remarkPlugins={[remarkGfm]}>{item.description}</ReactMarkdown>
```
No `rehype-raw` or `dangerouslySetInnerHTML` is used — this is safe by default,
because `react-markdown` v10 does **not** render embedded raw HTML unless a
`rehype-raw`-family plugin is explicitly added; raw HTML in the source markdown
is escaped/dropped, not executed.

**The risk**: `plan.md`/`requirements.md` content is AI-generated but can
transitively include user-controlled text (backlog item titles/descriptions get
fed into triage/plan prompts — see `session/backlog_context.go`). If a future
change adds `rehype-raw` (e.g. to support richer plan formatting, embedded
HTML tables, or diagrams) to satisfy a "nicer rendering" want, that reintroduces
an XSS vector for anything the AI pipeline echoes back verbatim from user input
into the rendered plan. `dompurify` is already a project dependency (used in
`web-app/src/lib/logs/logParser.ts`) — if raw HTML rendering is ever justified
for plan content, it must be piped through DOMPurify sanitization, not rendered
unsanitized.

**Design against**: reuse the exact `DescriptionSection.tsx` pattern (ReactMarkdown +
remarkGfm, no raw-HTML plugin) for plan content rendering. Do not add
`rehype-raw` without an explicit sanitization step.

---

## 5. Plan file path handling — traversal and missing-file precedent

**Confirmed in code**: two existing places read plan content from disk, with
different levels of rigor:

1. `session/backlog_review.go:715-722` (`readPlanFile`) — unconditionally
   `filepath.Join(artifactsDir, "plan.md")`, swallows read errors by returning
   `""`. **No validation that `artifactsDir` is within `project_plans/`** — it
   trusts `item.PlanArtifactsPath` as stored, which is itself only ever set by
   server-side triage code, not directly from client input. Safe today because
   nothing lets a client set this path arbitrarily, but a new RPC must preserve
   that invariant (never accept a client-supplied path/filename, only
   `item_id` → server-resolved path).
2. `server/services/backlog_service_lifecycle.go:636-640` (`ApprovePlan`) — does
   an explicit `os.Stat` and returns a clear `FailedPrecondition` connect error
   if the artifacts path is missing. **This is the pattern to replicate** for
   any new "get plan content" RPC: stat/read errors should surface as a
   typed, user-facing error (e.g. "plan file was moved or deleted — re-run
   planning"), not a 500 or a silently empty render.

**New risk surface specific to a content-serving RPC**: unlike `ApprovePlan`
(which only touches metadata), a new RPC that returns file *content* must also
guard against:
- Serving files outside the expected directory if `PlanArtifactsPath` is ever
  made partially client-influenced in a later change (defense in depth: resolve
  via `filepath.Clean` + a prefix check against the configured
  `project_plans/` root, not just trust the stored string).
- Large file / no size cap — nothing currently caps `plan.md` size before
  reading it fully into memory and serializing over ConnectRPC.
- Symlink escape — `os.Stat`/`os.ReadFile` follow symlinks; if plan artifact
  directories are ever created by a less-trusted process, a symlink inside
  `artifactsDir` could point outside `project_plans/`.

---

## 6. Autonomous / `skipPlanning` / `skipReviewGate` interaction with a more visible approval UI

**Confirmed in code**: three independent bypass flags exist across the item
schema/proto, each with distinct semantics:
- `skip_planning` (`session/repository.go:...`, gates `ApprovePlan`/spawn per
  `backlog_lifecycle.go:2531`: `if item.SkipPlanning || item.PlanApproved { continue }`)
- `skip_review_gate` (`proto/session/v1/backlog.proto:121,212,351`;
  `session/ent/schema/backlog_item.go:39`) — a **separate** flag, not the same
  as `skip_planning`, controlling a different gate.
- `PlanApproved`/`PlanApprovedAt` — the field this whole feature is about.

No single "autonomous mode" boolean was found in the schema; autonomy in this
codebase is expressed as a combination of these skip flags rather than one flag
(the requirements doc's phrase "autonomous-mode bypass inconsistency" maps to
this multi-flag reality, not a single toggle). **Risk**: once the UI adds a
prominent, persistent "plan approval status" indicator (Success Criterion #1 —
"no plan yet" / "pending review" / "approved" / "changes requested"), items that
never populate `PlanApproved` at all (because `skip_planning` or
`skip_review_gate` was set) will hit an ambiguous state: is "no plan yet" the
right label for "plan step was intentionally skipped," or does that read as a
stalled/broken item to the user? Get this wrong and the new indicator becomes
*more* confusing than the status quo it's meant to fix — exactly the reporter's
original complaint but relocated. **Design against**: the status indicator's
state machine needs an explicit "skipped" state distinct from "no plan yet" and
"pending review," derived from `skip_planning`/`skip_review_gate`, not just from
`PlanArtifactsPath`/`PlanApproved` truthiness.

---

## 7. Feature registry and e2e annotation debt

Per `.claude/rules/feature-registry.md` and `.claude/rules/e2e-test-conventions.md`:

- A precedent registry file already exists for the current RPC:
  `docs/registry/features/backend/backlog/approve-plan.json` (id
  `backlog:approve-plan`, `tested: true`, references
  `TestApprovePlan_HappyPath_SetsPlanApprovedAndTimestamp`,
  `TestApprovePlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition`,
  `TestTransitionGuard_ReadyToInProgress_RequiresPlanApprovedOrSkipPlanning`).
  A new `RejectPlan`/`RequestPlanChanges` RPC needs its **own** sibling file
  under `docs/registry/features/backend/backlog/`, not a mutation of this one.
- No `docs/registry/features/frontend/*plan*` file exists yet — any new
  frontend feature (plan content viewer, reject-with-reason form, approval
  status indicator) needs a new frontend registry entry with `markerFound`
  driven by a `// +feature:` comment in the component's first 10 lines.
- Any new `tests/e2e/*.spec.ts` file must start with `// @feature <ids>` (see
  the existing header in `plan-gate.spec.ts`:
  `// @feature backlog:transition-status, backlog:spawn-session`) — easy to
  forget when adding a fresh spec file rather than editing an existing one.
- After implementation, `make registry-generate` must be run and
  `docs/registry/coverage-gaps.json` checked for growth (per the registry
  rule) — a plan-content RPC or reject RPC shipped without a matching e2e test
  increases that count.

---

## 8. Miscellaneous / secondary risks

- **`ApprovePlan` has no rejection counterpart to invalidate on transition.**
  `backlog_service_lifecycle.go:598-602` already resets `PlanApproved`/`PlanArtifactsPath`
  on backward transitions to `idea`/`refining` — but does **not** reset any new
  `PlanRejected`/`PlanRejectionReason` fields (they don't exist yet). When adding
  those fields, this reset block must be extended in the same change, or a
  rejected item transitioned back to `refining` and then forward again could
  retain a stale rejection reason alongside a freshly-approved plan.
- **CSS**: `PlanArtifactsSection.tsx` currently imports styles from
  `../BacklogItemDetail.css` (a vanilla-extract `.css.ts` file, confirmed by the
  `.css` import path convention used throughout `web-app/src/components/backlog/detail/`).
  Any new plan-content-rendering component must add styles to a colocated
  `.css.ts` file per `.claude/rules/css-architecture.md`, not a new `.module.css`
  file, and must use `vars.*` tokens (no hardcoded hex, no `var(--undefined-token)`).
  A markdown-rendered plan will need code-block/table styling — pull existing
  tokens from `web-app/src/styles/theme.css.ts` rather than inventing new colors.
- **`readPlanFile` swallows all errors identically** (`session/backlog_review.go:715-722`
  returns `""` for both "file doesn't exist" and "permission denied" and any other
  `os.ReadFile` error) — reusing this helper as-is for a user-facing RPC would
  turn a real I/O error into a silent empty state indistinguishable from "no
  content yet." A content-serving RPC should not reuse `readPlanFile` unmodified;
  it should surface the specific error the way `ApprovePlan`'s `os.Stat` check
  already does.

---

## Summary of what to design against

1. **Optimistic-concurrency token** on plan content fetch, echoed back on
   approve/reject, to prevent stale-tab approvals racing plan regeneration.
2. **Always regenerate ent code via `session/ent/generate.go`'s exact command**
   (`--feature sql/upsert -mod=mod`) when adding rejection fields.
3. **Update `tests/e2e/plan-gate.spec.ts` in the same PR** if button copy/state
   changes, and add new gate coverage if the gate's scope widens beyond the
   queued-item path.
4. **No `rehype-raw`** for plan markdown rendering — replicate
   `DescriptionSection.tsx`'s ReactMarkdown+remarkGfm-only pattern; if raw HTML
   is ever needed, sanitize with the already-present `dompurify` dependency.
5. **Never accept a client-supplied file path** for plan content — resolve
   strictly server-side from `item_id`, follow the `ApprovePlan` `os.Stat`
   error-surfacing pattern, and don't reuse `readPlanFile`'s error-swallowing
   behavior for a user-facing RPC.
6. **Model "skipped" as a distinct status-indicator state**, not lumped into
   "no plan yet," given the multi-flag (`skip_planning`, `skip_review_gate`)
   bypass reality confirmed in code.
7. **New registry files, not edits to `approve-plan.json`**, for any new RPC;
   new spec files need `// @feature` headers; run `make registry-generate` and
   check `coverage-gaps.json` doesn't grow.
8. **Extend the existing backward-transition reset block** (`backlog_service_lifecycle.go:598-602`)
   to clear any new rejection fields, and use vanilla-extract `.css.ts` +
   `vars.*` tokens for any new rendering component's styles.
