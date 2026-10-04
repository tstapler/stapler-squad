# Research: Build vs. Buy — durable-guidance-request

## Context recap

Self-hosted, single-operator Go + ent + **SQLite** (confirmed: `go.mod` depends on
`github.com/mattn/go-sqlite3`; no Postgres driver — `lib/pq`/`pgx` — anywhere in
`go.mod`) + ConnectRPC + React tool. Feature: a durable "ask a human a structured
question, get notified asynchronously when answered" primitive, scoped to a backlog
item/session/standalone, rendered in 3 React views, delivered via the existing
event-bus/notification plumbing. No multi-tenant or compliance need; must extend, not
replace, the existing `ent`-backed durable-state pattern used for `BacklogStuckState`.

---

## 1. Existing OSS workflow-engine / durable-execution library for "durable ask-and-wait"

**Findings**: No workflow-engine dependency exists today — grep of `go.mod` for
`temporal|cadence|workflow|restate|hatchet|river|inngest` matched nothing except an
unrelated comment. Adopting Temporal (or Cadence, or a lighter durable-execution lib
like Restate/Hatchet/River) would mean:

- A **new external runtime dependency** (Temporal needs its own server + Postgres/MySQL
  + a worker process) or, for the lighter Go-native options (River, Hatchet), at minimum
  a new job-queue table/poller and a new mental model (workflows, activities, signals)
  layered on top of the ent schema this feature must already extend.
- The "durable ask-and-wait" shape this feature needs — persist a question row, let a
  human answer it whenever, wake up whoever's waiting via the existing event bus — is
  exactly what `BacklogStuckState`'s pattern already does for a structurally identical
  problem: a durable, resolve-in-place row keyed by a unique index
  (`item_id, reason`), atomically upserted via `INSERT ... ON CONFLICT`
  (`session/ent_repository_backlog.go:1929-1943`, `OnConflictColumns` on
  `backlogstuckstate.FieldItemID, backlogstuckstate.FieldReason`), with `notified_at`/
  `resolved_at` nullable timestamp fields doing the state-machine work a workflow engine
  would otherwise model as signals/steps.
- A workflow engine's core value — orchestrating multi-step, long-running, replay-safe
  business logic across process crashes — doesn't apply here: a `GuidanceRequest` is one
  row with two states (pending/answered), not a multi-step saga. There's nothing to
  "replay."

**Verdict: Not recommended.** The requirements doc itself mandates following the
`BacklogStuckState` precedent (Constraints: "Durable notify state must follow the
`BacklogStuckState` precedent... never an in-memory dedup map"), which already solves
the identical durability problem with zero new dependencies and proven SQLite
compatibility. A workflow engine would duplicate that pattern with a much heavier
runtime for a single-row, two-state entity.

---

## 2. Existing OSS library for the React form widget (yes/no | multiple-choice | short-answer)

**Findings**: `web-app/package.json` has **no form-builder or form-state library at
all** — no `react-hook-form`, `formik`, `react-jsonschema-form`, or `@rjsf/*` in either
`dependencies` or `devDependencies`. The existing form-ish UI (`@radix-ui/react-*`
primitives — dialog, tabs, accordion, tooltip) are headless UI primitives, not form
builders; `ApprovalRulesPanel.tsx` (762 lines) builds its own form state by hand with
plain React state, no library.

Evaluating whether to add one for this feature specifically:

- The requirement is "one component renders exactly one of three fixed field shapes"
  (a radio pair, a radio/select group, or a text input) — not a dynamic/nested/
  conditional schema. `react-jsonschema-form` and Formik+a schema renderer are built for
  arbitrary JSON-Schema-driven forms with validation chains, nested objects, and dynamic
  field arrays — none of which this feature has (out of scope per requirements.md:
  "Arbitrary custom-field/multi-step form schemas... only the three fixed types").
  Pulling in `react-jsonschema-form` to render three known variants is solving a
  much larger problem than exists.
- `react-hook-form` is lighter and a defensible general-purpose choice, but it exists to
  manage uncontrolled-input performance and validation-chain complexity across large
  forms. A three-variant switch component with one submit action doesn't have the form
  complexity (many fields, cross-field validation, re-render cost) that
  `react-hook-form` earns its keep on, and adding it as a new dependency for a single
  small component would be the first form library in the repo — a real ongoing
  maintenance/bundle-size commitment for a one-off.

**Verdict: Not recommended (adopt a new dependency). Recommended: hand-roll the
component**, following `ApprovalRulesPanel.tsx`'s existing house style (plain React
state, no form library) — a `switch` on `question.type` rendering a radio pair, a
radio/select group, or a text `<input>`/`<textarea>`, with one local "draft answer"
state variable and a single submit handler. This is a few dozen lines, not a
form-engine problem, and matches the codebase's existing precedent of not depending on
one anywhere else.

---

## 3. SaaS / managed "human-in-the-loop approval" API

**Findings**: Hosted approval-workflow APIs (e.g. generic "human-in-the-loop" or
approval-step SaaS products) provide a request/approve/deny primityive and a hosted
widget or Slack/email integration, but:

- They have **zero knowledge of this app's domain model** — a "backlog item," "session,"
  or "triage run" scope is meaningless to a generic hosted approval widget. Every
  answer would still need to be round-tripped back into this app's own `ent` schema and
  event bus to be useful to a resuming session — meaning the SaaS integration would sit
  *on top of* the exact same durable-storage-plus-notification work this feature already
  has to build, not instead of it.
- This is a single-operator, self-hosted tool with **one answerer** (Tyler, via the web
  UI only — per requirements.md's Answerer decision, no MCP/agent-submitted answers in
  v1). A SaaS approval product's value proposition (multi-approver routing, audit trail
  for compliance, org-wide policy) targets a multi-tenant/multi-approver setting this
  tool explicitly doesn't have.
- It would add an external network dependency (webhook receiver or polling) for a
  feature whose whole point is to work for **unattended, possibly offline-for-a-while**
  agent runs — an extra third-party service in the loop is an extra availability
  dependency for no compensating benefit.

**Verdict: Not recommended.** No hosted approval API can express "backlog item" or
"session" scope, so integrating one would still require building this feature's entire
domain-storage-and-notification layer anyway, with an added external dependency and no
matching multi-approver/compliance need to justify it.

---

## 4. LLM-generated vs. battle-tested library — algorithmic pieces

### (a) Atomic upsert / unique-key dedup for the guidance-request row

**Findings**: This is a direct structural rerun of `BacklogStuckState`'s already-shipped,
already-tested pattern:

- Schema: a unique 2+-column index as the resolve-in-place key (here, plausibly
  `(scope_type, scope_id, ...)` or a single `id`-based create-then-guard, depending on
  whether a `GuidanceRequest` should ever be "reopened in place" the way a stuck-state
  row is — see Plan phase) — `session/ent/schema/backlog_stuck_state.go:85-92`.
- Repository method: `tx.<Entity>.Create()....OnConflictColumns(...).Update(func(u
  *ent.<Entity>Upsert) {...}).Exec(ctx)` inside an explicit `r.client.Tx(ctx)` —
  `session/ent_repository_backlog.go:1922-1943`. The schema's own comment documents why
  this must be a *plain* unique index rather than a partial one: "a plain unique index
  cannot be an `OnConflictColumns` target for an append-only design on SQLite, since
  NULLs are distinct" (`backlog_stuck_state.go:24-25`) — a SQLite-specific gotcha
  already paid for once; reusing the pattern avoids re-discovering it.

**Verdict: Recommended — reuse the pattern verbatim, not a new library.** This is
exactly the `INSERT ... ON CONFLICT` dedup precedent the requirements doc's Constraints
section already mandates reusing (AC5). No third-party dedup/idempotency library
(e.g. an idempotency-key middleware package) is warranted — `ent`'s native
`OnConflictColumns` plus a unique index already gets this for free at the SQLite level.

### (b) Per-scope pending-count cap check (AC7)

**Findings**: This is a single `COUNT(*) WHERE scope_type = ? AND scope_id = ? AND
status = 'pending'` query, checked before insert. No existing cap-checking library or
pattern in this repo (the requirements doc's own Rabbit Holes section confirms: "no
existing per-item cap pattern exists — the WIP cap is board-wide, not per-item").
Race-safety needs a transaction (count-then-insert inside one `tx`, or a `CHECK`-style
guard at insert time) — not a distributed rate-limiter or semaphore library, since this
is single-process, single-SQLite-file state, not a distributed-systems problem.

**Verdict: Recommended to hand-write.** A counting query wrapped in the same
transaction as the insert is a few lines, directly testable, and matches the
requirements doc's own framing ("likely not worth a library"). No library
(e.g. `golang.org/x/time/rate`, a distributed limiter) fits a single-SQLite-file,
single-process count check.

---

## 5. Fork or adapt an existing in-repo component/schema

**Findings, confirming the prior codebase survey (requirements.md's Alternatives
Considered) via direct read of both files:**

- **`ApprovalRulesPanel.tsx`** (`web-app/src/components/sessions/ApprovalRulesPanel.tsx`,
  762 lines) — its `ApprovalRulesPanelProps` interface and surrounding logic operate on
  global tool-approval rules (glob/regex match on tool name + args), with **no
  `item_id`/`session_id` scoping field anywhere in the component or its backing schema**.
  Confirmed: poor structural fit, as the prior survey found — this is global policy
  configuration, not a per-item/session question-and-answer record.

- **`PendingApproval`** (`server/services/approval_store.go:21-53`) — structurally the
  closest existing precedent: it's keyed by `SessionID`, carries a `CreatedAt`/
  `ExpiresAt` pair, and has an explicit `Orphaned bool` field for the exact "session/
  process died, can this still be resolved" question this feature also has to answer.
  **However, direct inspection of `ApprovalStore` (the same file, lines 75-461) shows
  its durability model is the opposite of what's needed:**
  - State lives in an in-memory `map[string]*PendingApproval` (`pending`,
    `bySession`), not `ent`/SQLite.
  - Persistence to disk (`persistToDiskLocked`, lines 344-396) is a flat JSON file
    (`pending_approvals.json`), written via temp-file-then-rename — not a queryable
    durable store, and not the `ent`-backed model this feature's Constraints
    section requires.
  - Critically, **an approval loaded from disk after a restart is marked
    `Orphaned: true` and can only be denied/removed** (`loadFromDisk`, lines 398-455;
    `Resolve`, lines 211-240: "Orphaned approvals have no live HTTP connection --
    just remove the record"). The answer delivery mechanism (`decisionCh`, an
    in-memory buffered channel, line 52) is explicitly `nil` for any approval that
    survived a restart (line 444). In other words, `PendingApproval` is disk-backed
    only for *cleanup bookkeeping* — it cannot actually deliver an answer to anything
    once the process that created it is gone, which is the exact opposite of this
    feature's core requirement ("a question created by a now-dead/paused session is
    still answerable... durably retrievable by a fresh session").

**Verdict on forking**: **Not recommended to fork wholesale, for either candidate** —
confirming and sharpening the prior survey's conclusion. `ApprovalRulesPanel` fails on
scoping (as already found); `PendingApproval`/`ApprovalStore` fails on durability
despite *looking* durable (JSON-file-backed) — its restart story is "delete it," not
"answer it later." The right precedent to fork is `BacklogStuckState` (§4a above),
which is genuinely `ent`/SQLite-backed with atomic upsert and survives restart with the
row still live and actionable — matching this feature's actual durability bar
("mirrors the stuck-review durability bar" per requirements.md's Success Metrics).

---

## Summary of verdicts

| # | Question | Verdict |
|---|---|---|
| 1 | Workflow-engine library (Temporal/Cadence/lighter durable-execution lib) | Not recommended — `BacklogStuckState`'s existing ent-backed upsert/resolve-in-place pattern already solves this single-row, two-state problem with zero new runtime dependencies |
| 2 | React form-builder library (react-hook-form / Formik / react-jsonschema-form) | Not recommended — none is currently a dependency; three fixed field shapes don't need a schema-driven form engine. Hand-roll, matching `ApprovalRulesPanel.tsx`'s existing no-library house style |
| 3 | SaaS human-in-the-loop approval API | Not recommended — can't express "backlog item"/"session" scope; would still require building this feature's own storage/notification layer on top, plus an unneeded external dependency for a single-operator tool |
| 4a | Atomic upsert / dedup library | Not recommended — reuse `BacklogStuckState`'s `OnConflictColumns` + unique-index pattern verbatim (`session/ent_repository_backlog.go:1929-1943`) |
| 4b | Per-scope pending-cap library | Not recommended — a transactional `COUNT(*)` query is sufficient; no rate-limiter/semaphore library fits single-process SQLite state |
| 5 | Fork existing component/schema | Not recommended for either candidate — `ApprovalRulesPanel` lacks item/session scoping (confirmed); `PendingApproval`/`ApprovalStore` looks durable (JSON-file-backed) but marks every restart-surviving row `Orphaned` and can only deny/delete it, never deliver an answer — the opposite of this feature's durability requirement. Fork `BacklogStuckState` instead |
