# UX Design: Webhook / Event-Driven Trigger and Callback Configuration

SDD Phase 3 design artifact for `webhook-triggers`. Extends `research/ux.md`'s
findings into concrete wireframes, interaction flows, and testable acceptance
criteria. Field/type names below are taken directly from
`implementation/plan.md`'s Domain Glossary and Epics 1–8 — not invented.

Precedent extended: `web-app/src/components/sessions/ApprovalRulesPanel.tsx`
(list/tabs/toggle/badge/mobile-FAB/`aria-live` shapes) and
`web-app/src/components/rules/RuleBuilderForm.tsx` (conditional field-set-by-type
form). New components per the plan's Phase 7: `TriggersPanel.tsx`,
`TriggerExecutionHistory.tsx`, `TriggerTestModal.tsx`, `CallbackSettings.tsx`,
plus a session-card attribution badge (`SessionCard.tsx`/`SessionDetail.tsx`).

---

## Surface inventory

| # | Surface | New component | Backend it reads/writes |
|---|---|---|---|
| 1 | Trigger list | `TriggersPanel.tsx` | `ListWorkflows`, `UpdateWorkflow` (toggle) |
| 2 | Trigger builder (create/edit) | `TriggersPanel.tsx` form section (extends `RuleBuilderForm.tsx` pattern) | `CreateWorkflow`, `UpdateWorkflow` |
| 3 | Trigger execution history | `TriggerExecutionHistory.tsx` | `ListTriggerFireEvents` |
| 4 | Test/dry-run trigger | `TriggerTestModal.tsx` | `TestTrigger` |
| 5 | Outbound callback config | `CallbackSettings.tsx` | `GetCallbackConfig`, `UpdateCallbackConfig` |
| 6 | Session trigger-attribution badge | `SessionCard.tsx` / session detail view | reads existing `Session.WorkflowId` |

6 surfaces total (the 4 named in the task brief, with the list/config panel
split into its two constituent screens — list and builder — since they have
materially different wireframes, plus the cross-cutting attribution badge
that AC6 requires).

---

## Surface 1: Trigger List (`TriggersPanel`)

### Wireframe — desktop

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Triggers                                          [Test All] [+ Add Trigger]│
│ Inbound events that create sessions automatically — reviewed the same way   │
│ as manually created sessions.                                               │
├─────────────────────────────────────────────────────────────────────────────┤
│ [Search by name, slug, repo…]                                               │
│ (All 7) (Cron 2) (GitHub Push 3) (Webhook 2)                                │
├─────────────────────────────────────────────────────────────────────────────┤
│ Name ▾      │Type          │Last fired      │Received/Matched│Enabled│      │
├─────────────┼──────────────┼────────────────┼─────────────────┼───────┼─────┤
│ nightly-audit│🕐 Cron       │3h ago ✅        │  —              │ [ON]  │ ⋯  │
│ jira-ticket │🪝 Webhook     │12m ago ✅       │ 41 / 9          │ [ON]  │ ⋯  │
│ pr-review   │🐙 GitHub Push │2d ago ⚠️        │  6 / 6          │ [ON]  │ ⋯  │
│ old-sync    │🪝 Webhook     │Never fired      │  0 / 0          │ [OFF] │ ⋯  │
│                                     (row grayed — disabled)                 │
│ stale-hook  │🪝 Webhook     │5m ago 🛑 3 rejected│ 12 / 0        │ [ON]  │ ⋯  │
└─────────────────────────────────────────────────────────────────────────────┘
  5 triggers

[aria-live="polite" hidden span: "Trigger enabled." / "Trigger disabled." /
 "Webhook rejected: invalid signature — 3 rejected requests in the last hour."]
```

- `Last fired` status glyphs mirror the five-state model
  (`implementation/plan.md`'s `TriggerFireOutcome`): ✅ `fired_success`,
  ⚠️ `fired_failed` (last fire matched but session creation errored), 🛑 a
  `rejected`-count badge shown inline when > 0 in the trailing 24h (research/ux.md
  §4's "treat like a failed-login counter, not routine noise" — this is why it's
  on the list row, not buried in the drill-down).
- `Received/Matched` is the on-demand-by-default counter from research/ux.md §2
  ("N received / M matched") — `no_match` events are *not* shown as rows here,
  only rolled into this count, to avoid noise from routine non-matching pushes.
- Clicking a row (not a control) opens Surface 3 (execution history) for that
  `Workflow`.
- `⋯` per-row menu: Edit, Test, Duplicate, Delete — collapses the four icon
  buttons `ApprovalRulesPanel.tsx` places inline (Edit/Delete) into one menu on
  this panel, since Trigger rows carry a 3rd action (Test) the Rules panel
  doesn't have; each menu item is still a real `<button>` with a text label
  when the menu is open (not icon-only), satisfying the keyboard/AT
  requirement without needing five separate 44px targets per row.

### Wireframe — mobile (stacked cards, per research/ux.md's "no horizontal
scroll if >3-4 columns" rule — this table has 5)

```
┌───────────────────────────────┐
│ Triggers              [≡ filter]│
├───────────────────────────────┤
│ ┌───────────────────────────┐ │
│ │ jira-ticket      🪝 Webhook│ │
│ │ Last fired: 12m ago ✅     │ │
│ │ 41 received / 9 matched   │ │
│ │           [ON]      ⋯     │ │
│ └───────────────────────────┘ │
│ ┌───────────────────────────┐ │
│ │ stale-hook       🪝 Webhook│ │
│ │ Last fired: 5m ago         │ │
│ │ 🛑 3 rejected (24h)         │ │
│ │ 12 received / 0 matched   │ │
│ │           [ON]      ⋯     │ │
│ └───────────────────────────┘ │
│                                │
│                          (+) ← 56px FAB, "Add trigger"
└───────────────────────────────┘
```

- Mirrors `ApprovalRulesPanel.tsx`'s `mobileAddFab` +
  `headerButtonsHiddenOnMobile` split exactly: "Add Trigger" becomes the FAB;
  "Test All" (secondary, low-frequency) hides on mobile entirely rather than
  wrapping into a second toolbar row.
- Each card's `[ON]`/`⋯` controls are ≥44×44px tap targets (mobile convention,
  `docs/tasks/mobile-ux-improvements.md:296-297`).

### Interaction flow

1. User lands on `TriggersPanel` (new nav entry alongside Approval Rules).
2. `ListWorkflows` loads rows; loading state matches `ApprovalRulesPanel`'s
   `loading`/`empty` classes.
3. **Empty state** (zero triggers of any type): reuses the `empty` div with
   contextual copy — *"Triggers let external events (a GitHub push, a
   schedule, a webhook) create a session automatically, reviewed the same way
   as anything you create by hand. [Add your first trigger]"* — mirrors
   `ApprovalRulesPanel.tsx:471-503`'s per-filter contextual empty copy, not a
   bare "no data."
4. **Empty state per type tab** (e.g. `Cron` tab selected, zero cron
   triggers exist): *"No cron triggers yet. [+ Add Cron Trigger]"* — same
   per-tab-empty-state pattern as the Rules panel's source tabs.
5. Toggle click → optimistic UI flip + `UpdateWorkflow` call; on failure,
   revert the toggle and show a toast/banner: *"Couldn't disable jira-ticket:
   <server error>. [Retry]"* — never a silent revert.
6. Row click → navigates to Surface 3 for that trigger.
7. `⋯ → Delete` → confirmation dialog (matches existing delete-confirmation
   pattern elsewhere in the app) — *"Delete trigger jira-ticket? This does
   not affect sessions it already created."* [Cancel] [Delete].

---

## Surface 2: Trigger Builder (create/edit)

Extends `RuleBuilderForm.tsx`'s conditional-field-set-by-type shape. Opens
inline below the list (same `formSection`/`role="dialog"` pattern as
`#rule-builder`), not a separate route.

### Wireframe — type selector + shared fields

```
┌─ Add Trigger ──────────────────────────────────────── [×] ─┐
│ Name: [nightly-audit____________]                          │
│                                                              │
│ Trigger type:                                                │
│  (•) Cron        ( ) GitHub Push        ( ) Webhook          │
│                                                                │
│ ── fields below change based on the selected type, and are ──│
│ ── announced via aria-live when the type radio changes ──────│
│                                                                │
│ <fieldset legend="Cron schedule">                             │
│   Schedule (5-field cron): [0 9 * * *________]  Next: 09:00  │
│   tomorrow (UTC-5)                                            │
│ </fieldset>                                                   │
│                                                                │
│ Session target: (•) New worktree  ( ) Existing directory       │
│ Repository path: [/home/tstapler/Programming/stapler-squad]   │
│                                                                │
│ Prompt template (Go text/template):                           │
│ ┌────────────────────────────────────────────────────────┐  │
│ │ Run the nightly dependency audit and open a session      │  │
│ │ summarizing any CVEs found.                               │  │
│ └────────────────────────────────────────────────────────┘  │
│                                                                │
│ ▸ Advanced: chain to another trigger on completion            │
│                                                                │
│               [Send Test Event]   [Cancel]  [Save Trigger]    │
└────────────────────────────────────────────────────────────┘
```

### Type-specific field sets (each a real `<fieldset>`/`<legend>`, per
research/ux.md §3's screen-reader requirement — switching the `type` radio
must announce the newly revealed group, not just visually swap it)

**`github_push`**
```
<fieldset legend="GitHub push match">
  Repository (owner/repo): [tstapler/stapler-squad_______]
  Branch:                  [main_________________________]
  Webhook secret: [ Generate new secret ]  (shown once, copy-to-clipboard;
    never re-displayed after this dialog closes — mirrors CallbackSettings'
    masking, Surface 5)
  Receiver URL (read-only, for pasting into GitHub's webhook config):
    https://<host>/webhooks/github            [Copy]
</fieldset>
```

**`webhook`**
```
<fieldset legend="Webhook match">
  Slug:          [jira-ticket_________]  → /webhooks/jira-ticket [Copy]
  Event filter:  [issue_created________]  (matches payload.event exactly)
  Label filter (optional): [urgent_____]  (matches payload.labels[])
  Shared secret: [ Generate new secret ]  (same one-time-reveal pattern)
</fieldset>
```

- Slug field validates uniqueness client-side against the already-loaded
  `ListWorkflows` result before submit (fast-fail), and shows the server's
  `webhook_slug` uniqueness error inline on the field if a race loses:
  *"That slug is already in use by another trigger. [Suggest: jira-ticket-2]"*

**Prompt template validation (all three types)**: on blur, a lightweight
client-side Go-template syntax check (balanced `{{ }}`) flags obvious typos
before submit; the authoritative check is server-side
(`WorkflowService.CreateWorkflow`/`UpdateWorkflow`'s `template.Parse`, Task
3.1.1b) — a parse error there returns `connect.CodeInvalidArgument` and is
shown inline under the textarea: *"Template error: unexpected `}}` at
position 42. [Jump to error]"* — this is the "catch typos at config-save
time, not fire time" requirement (plan.md pitfalls §4), surfaced as a
non-blocking-to-navigate, blocking-to-save inline error (not a modal, not a
toast that disappears).

### Pipeline chaining sub-section (`▸ Advanced`, collapsed by default —
low-frequency, high-complexity option per Jakob Nielsen progressive
disclosure)

```
▾ Advanced: chain to another trigger on completion
  When a session created by this trigger reaches "done," automatically
  fire another trigger with this session's output available in its prompt.

  Next trigger: [Select a trigger... ▾]  (dropdown of existing Workflows;
                                            excludes this trigger itself —
                                            see cycle guard below)
  ⚠ Chain depth cap: 5 hops (this repo's runaway-loop backstop — a session
    created this way cannot itself chain more than 4 further times)
```

- **Cycle guard, client-side UX**: if the user selects a `next trigger` that
  would form a direct A→B→A cycle (detectable client-side from the already
  loaded trigger list's existing `next_workflow_id` links), the dropdown
  shows the option grayed out with inline text: *"Selecting this would create
  a loop with <trigger name> — choose a different trigger."* The
  server-side `maxChainDepth` cap (Epic 6.3) is the authoritative backstop for
  cycles the UI can't detect (e.g. indirect A→B→C→A), so this is a UX
  trust-builder, not the enforcement mechanism.

### Interaction flow

1. `+ Add Trigger` (header button or mobile FAB) → form opens, `type` defaults
   unselected (forces an explicit choice — no accidental cron-by-default).
2. Selecting a type radio reveals that type's `<fieldset>`; an `aria-live`
   region announces *"Webhook fields shown."*
3. `[Send Test Event]` → opens Surface 4 (dry-run) pre-populated with the
   in-progress form's current field values (not yet saved) — lets the user
   validate the template *before* committing, matching Zapier's pattern cited
   in research/ux.md §1. Available even on an unsaved/new trigger, since the
   dry-run doesn't require a persisted `Workflow` row (it can run
   `RenderTriggerPrompt` against the client-held template + a sample payload
   without a `TestTrigger(workflow_id, ...)` round trip — or, if the RPC
   requires a persisted ID, the button is disabled with a tooltip *"Save the
   trigger first to test it"* — resolve which per the RPC's actual signature
   in Task 7.2.1d; either way the button is never silently absent).
4. `[Save Trigger]` → `CreateWorkflow`/`UpdateWorkflow`; on success, form
   closes, list refreshes, new row briefly highlighted (2s fade), `aria-live`
   announces *"Trigger nightly-audit created."*
5. On save failure (validation or server error), form stays open, error shown
   inline near the offending field (not a generic top-of-form banner when the
   error is field-specific) plus a top-of-form summary banner when it isn't
   (e.g. a 500).
6. `[Cancel]` / `Escape` → closes with no changes, matching
   `ApprovalRulesPanel.tsx:137-144`'s existing `Escape`-to-close behavior; if
   the user has unsaved edits, a lightweight confirm (*"Discard changes?"*)
   prevents silent data loss — same standard as any form with typed content.

---

## Surface 3: Trigger Execution History (`TriggerExecutionHistory`)

### Wireframe — desktop (opened by clicking a trigger row on Surface 1)

```
┌─ jira-ticket · 🪝 Webhook ─────────────────────────── [Edit] [Test] [×] ─┐
│ /webhooks/jira-ticket · event=issue_created · label=urgent               │
│ 41 received · 9 matched · 3 rejected (last 24h)                          │
├────────────────────────────────────────────────────────────────────────┤
│ [x] Show non-matching events   (unchecked by default — research/ux.md §2)│
├────────────────────────────────────────────────────────────────────────┤
│ Status         │ Time        │ Detail                    │ Session      │
├────────────────┼─────────────┼────────────────────────────┼──────────────┤
│ ✅ Fired         │ 12m ago     │ PROJ-142 "Fix login bug"  │ → session-a1 │
│ 🛑 Rejected      │ 34m ago     │ Signature mismatch        │ —            │
│ ⚠️ Fired, failed │ 2h ago      │ WIP limit reached          │ —            │
│ ✅ Fired         │ 5h ago      │ PROJ-140 "Update docs"    │ → session-9f │
│ ▫ No match       │ 6h ago      │ event=issue_closed (≠ filter)│ —          │  ← only visible
│                                                                              │    when checkbox on
└────────────────────────────────────────────────────────────────────────┘
```

- Five distinct badges match `implementation/plan.md`'s `TriggerFireOutcome`
  1:1, plus the derived `disabled` state shown on Surface 1 only (a disabled
  trigger doesn't evaluate events, so it has no new fire-event rows to show
  here): `fired_success` (✅ green, links to session), `fired_failed`
  (⚠️ amber — distinct from rejected, since this means the trigger *matched*
  but session creation errored), `rejected` (🛑 red — signature/malformed,
  never mixed visually with `no_match`), `no_match` (▫ gray, collapsed behind
  the checkbox by default).
- Clicking a `✅ Fired` row's session link navigates to that session's detail
  view, which shows the reciprocal attribution badge (Surface 6) — the
  "symmetric navigation" research/ux.md §5 calls for.
- `Detail` column for `rejected` never echoes the invalid signature itself
  (would leak information useful to an attacker probing the endpoint) — only
  the reason category (*"Signature mismatch," "Malformed JSON," "Unknown
  slug"*).

### Mobile — stacked cards (same per-column-collapse rule as Surface 1)

```
┌───────────────────────────────┐
│ jira-ticket          [Edit][×]│
│ 41 recv · 9 matched · 3 reject│
├───────────────────────────────┤
│ ✅ Fired · 12m ago              │
│ PROJ-142 "Fix login bug"       │
│ → session-a1                   │
├───────────────────────────────┤
│ 🛑 Rejected · 34m ago           │
│ Signature mismatch             │
└───────────────────────────────┘
```

### Interaction flow / edge cases

- **Zero events ever** (webhook receiving nothing): empty state reads *"No
  events received yet. Once <slug> receives a request at
  /webhooks/jira-ticket, activity appears here."* — this is the case
  research/ux.md §2 flags as indistinguishable from "correctly filtering" if
  not labeled explicitly; the received-count of `0` in the header (`0
  received · 0 matched`) is the disambiguator.
- **Rejected count rising** (possible probing/misconfigured sender): if
  rejected count in the last hour crosses a small threshold (e.g. 5), a
  banner appears at the top of this panel: *"⚠ 6 rejected requests in the
  last hour — check that the sender is using the correct secret."* No
  auto-disable (out of scope per requirements' non-goals), but the signal is
  surfaced, not buried in a scrollable log a user has to notice on their own.
- **WIP-limit-blocked fire** (`fired_failed`, "WIP limit reached"): detail
  text links to wherever the WIP-limit / concurrent-session-count is
  surfaced elsewhere in the app (if such a view exists) so the user
  understands *why*, not just *that* it failed — satisfies research/ux.md
  §5's "runaway trigger is caught by the human" emotional job.
- **Chain-depth cap hit** (`fired_failed`, "chain depth exceeded"): same
  treatment — detail text explicitly states the cap (*"Chain depth exceeded
  (max 5) — this session's chain stops here."*), not a generic failure
  string, so the user understands this is an intentional backstop, not a bug.

---

## Surface 4: Test / Dry-Run Trigger (`TriggerTestModal`)

### Wireframe

```
┌─ Test jira-ticket ──────────────────────────────────── [×] ─┐
│ Send a sample payload through this trigger's matching and    │
│ template rendering — no session is created.                  │
│                                                                │
│ Sample payload (JSON):                                        │
│ ┌────────────────────────────────────────────────────────┐  │
│ │ {                                                          │
│ │   "event": "issue_created",                                │
│ │   "labels": ["urgent"],                                    │
│ │   "issue": { "key": "PROJ-1", "summary": "fix it" }        │
│ │ }                                                            │
│ └────────────────────────────────────────────────────────┘  │
│ [Use last received payload ▾]  (populated from most recent    │
│  TriggerFireEvent, if any — saves retyping a realistic sample)│
│                                                                │
│                                    [Run Test]                 │
│ ──────────────────────────────────────────────────────────  │
│ Result:                                                        │
│  ✅ Would match · label_filter "urgent" ✓ · event "issue_created" ✓│
│                                                                  │
│  Rendered prompt:                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │ Triage PROJ-1: fix it                                       │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                  │
│                                                    [Close]      │
└──────────────────────────────────────────────────────────────┘
```

### Non-matching result

```
Result:
  ▫ Would not match · event "issue_closed" ≠ configured "issue_created"
  No prompt would be rendered — this event would appear as "No match" in the
  execution history, not create a session.
```

### Malformed payload result

```
Result:
  ⚠ Invalid JSON at line 3, column 12 — fix the sample payload above and
  run the test again. [Jump to error]
```

### Interaction flow

- Modal is focus-trapped, closes on `Escape` and the `[×]`, matching the
  existing `role="dialog"` convention on `#rule-builder`
  (`ApprovalRulesPanel.tsx:638`) per research/ux.md §3.
- `[Run Test]` calls the `TestTrigger(workflow_id, sample_payload)` RPC
  (Task 7.2.1d) which runs match + `RenderTriggerPrompt` **without** calling
  `CreateSession` — this is the load-bearing guarantee the modal's own
  subtitle states explicitly ("no session is created") so a user isn't
  afraid to click it.
- Result region is `aria-live="polite"` so a screen-reader user gets the
  result announced without needing to re-focus into the modal
  (research/ux.md §3).
- A template-rendering error (missing field the template references) shows
  the same category of message as the save-time validation on Surface 2:
  *"Template error: field `.issue.priority` not found in this payload."* —
  this is the exact case plan.md's Story 3.1.1 flags as "fails cleanly, not
  a 500," surfaced here as the primary place a user discovers it before ever
  going live.

---

## Surface 5: Outbound Callback Config (`CallbackSettings`)

### Wireframe

```
┌─ Callback Notifications ──────────────────────────────────┐
│ POST a JSON payload to an external URL when these events    │
│ happen. Delivery is best-effort (3 retries) and never blocks│
│ or is blocked by the underlying session/backlog action.     │
│                                                                │
│ On session complete                                            │
│  URL: [https://exam••••••••••••••••••••.com/hook] [Edit] [Clear]│
│  ✅ Configured                                                  │
│                                                                  │
│ On session stale                                                │
│  URL: [ Not configured ]                          [+ Add]      │
│                                                                  │
│ On queue item created                                           │
│  URL: [ Not configured ]                          [+ Add]      │
│                                                                  │
│                                              [Send Test Ping]   │
└────────────────────────────────────────────────────────────┘
```

- Masking convention matches the plan's explicit `SlackConfigProto` /
  `SlackNotificationSettings.tsx` precedent (Epic 5.1's proto comment: "never
  echo the URL"): the RPC (`GetCallbackConfig`) returns only
  `*_configured: bool` booleans, never the plaintext URL, so the UI can only
  ever render a masked placeholder + "Configured" state — it structurally
  cannot leak a previously-saved URL back onto screen. `[Edit]` opens a fresh
  input (empty, not pre-filled with the masked value) to set a *new* URL,
  matching how a masked secret field normally behaves (write-only after
  first save).
- `[Clear]` removes the URL (`UpdateCallbackConfig` with the field set to
  empty string) with an inline confirm — *"Stop sending on-session-complete
  callbacks?"*
- `[Send Test Ping]` fires a synthetic payload to whichever URLs are
  configured and reports per-URL result inline (*"on_session_complete: 200
  OK (312ms)" / "on_session_stale: timed out after 5s"*) — the callback
  analogue of Surface 4's dry-run, giving the same pre-commit trust-building
  research/ux.md calls out as the single highest-leverage pattern, applied to
  the outbound side too.

### Interaction flow / edge cases

- Entering a non-URL string → inline validation error before submit:
  *"Enter a valid https:// URL."*
- Save failure (network/server error) → inline error under the field, field
  retains the user's typed value (never silently cleared on failure).
- No "delivery failed" list is shown here by default — per FR9's requirement
  that failures aren't silently dropped, but plan.md's Observability Plan
  routes this through logs (`log.Warn`, URL redacted), not a new UI list in
  this pass. This panel's `[Send Test Ping]` is the interactive substitute a
  user has today; a fuller delivery-history view is a plausible fast-follow,
  called out here as a known gap rather than silently omitted.

---

## Surface 6: Session Trigger-Attribution Badge

### Wireframe — session card (list context)

```
┌─────────────────────────────────────────┐
│ ● fix-login-bug                          │
│ tstapler/stapler-squad · main            │
│ 🔗 Triggered by: jira-ticket (webhook)    │  ← new badge, links to Surface 3
│ Status: In Review                         │
└─────────────────────────────────────────┘
```

### Wireframe — session detail view

```
┌─ fix-login-bug ────────────────────────────────────────────┐
│ 🔗 Triggered by: jira-ticket (webhook) · fired 12m ago       │
│    → View trigger config    → View this firing's detail      │
│ ...                                                            │
└────────────────────────────────────────────────────────────┘
```

- Reads the existing `Session.WorkflowId` field (no new field needed, per
  plan.md's Pattern Decisions table — "Session-level source attribution"
  row) and the `Workflow.TriggerType`; renders nothing (no badge) when
  `WorkflowId` is empty or the `Workflow`'s `trigger_type == "manual"` — a
  manually created session shows no badge at all, which is itself the signal
  that it wasn't automation (AC6's "not indistinguishable from manually
  created ones" cuts both ways: triggered sessions are marked, manual
  sessions are conspicuously unmarked).
- Two links from the detail view: one to the trigger's config (Surface 2,
  edit mode) and one to the *specific* `TriggerFireEvent` row on Surface 3
  that created this session — the symmetric navigation research/ux.md §5
  calls for (trigger-log-row → session already exists per Surface 3; this is
  the reverse direction).

---

## Cross-cutting error / edge-case handling summary

| Scenario | Where surfaced | User-visible message | Exit path |
|---|---|---|---|
| Malformed inbound webhook payload | Surface 3 (`rejected` row) + Surface 4 (test result) | "Invalid JSON" / category-only reason, never raw parse internals | Edit trigger (Surface 2) or re-test (Surface 4) |
| Bad/missing HMAC signature | Surface 1 (rejected-count badge) + Surface 3 (`rejected` row, banner if count spikes) | "Signature mismatch" — never echoes the received/expected signature | Edit trigger to rotate secret (Surface 2) |
| WIP-limit blocked a trigger fire | Surface 3 (`fired_failed` row) | "WIP limit reached" with link to explain | No action needed — self-resolves as WIP frees up; user can disable the trigger meanwhile (Surface 1 toggle) |
| Chain-depth cap hit | Surface 3 (`fired_failed` row on the terminal item's chain-fire attempt) | "Chain depth exceeded (max 5)" | None needed — intentional backstop, not an error state requiring user action |
| Template render/parse error | Surface 2 (save-time, blocking) + Surface 4 (test-time, non-blocking) | "Template error: <field/position>" | Fix template inline, re-save or re-test |
| Duplicate webhook slug | Surface 2 (save-time) | "That slug is already in use" + suggested alternative | Pick a different slug, retry save |
| Trigger toggle fails | Surface 1 | Toast: "Couldn't disable <name>: <reason>. [Retry]" | Retry button; toggle visually reverts, never left in an ambiguous state |
| Callback URL save fails | Surface 5 | Inline error under field, value retained | Retry save |
| Cycle in chain config | Surface 2 (client-side guard, best-effort) + Surface 3 (`fired_failed`, server-side cap as authoritative backstop) | "Selecting this would create a loop" / "Chain depth exceeded" | Choose a different next-trigger, or accept the depth cap as the safety net |

Every row above has a next action available from the same screen — no state
requires navigating away to find out what to do next (the "no dead ends"
acceptance criterion below).

---

## UX Acceptance Criteria

Numbered for traceability back to this doc; each is human-testable.

### Task efficiency

1. **UX-AC1**: A user can create a fully configured `webhook` trigger
   (name, slug, event filter, prompt template, secret) and see it appear
   enabled in the list in ≤ 6 steps: open panel → Add Trigger → select type
   → fill fields → Save → see row in list.
2. **UX-AC2**: A user can enable/disable an existing trigger in **1 click**
   (the toggle), from the list view, with no confirmation dialog required
   for *disable* (low-risk, reversible) but a confirmation required for
   *delete* (irreversible w.r.t. config, though prior sessions are
   unaffected).
3. **UX-AC3**: A user can dry-run a trigger's template rendering in ≤ 3
   steps from the builder form: click "Send Test Event" → (optionally load
   last-received payload) → Run Test — without ever having saved the
   trigger first if the `TestTrigger` RPC supports unsaved templates (Task
   7.2.1d dependent; if it requires a saved `workflow_id`, the flow is Save →
   Test, and the button must never be silently disabled without the
   "Save the trigger first" tooltip explaining why).
4. **UX-AC4**: A user can determine "is this trigger dead or just not
   matching anything" without opening any detail view — the list row's
   `Received/Matched` count and last-fired status are visible directly in
   Surface 1's table/cards.
5. **UX-AC5**: A user can trace a session back to the trigger that created it
   in **1 click** (the attribution badge on the session card/detail), and
   from the trigger's execution history forward to the exact session it
   created, also in **1 click** — symmetric, bidirectional navigation.

### Error and edge-case exits

6. **UX-AC6**: Every error state listed in the cross-cutting table above
   shows a specific, actionable message (never a bare "Error" or raw
   stack/exception text) and offers a concrete next action from the same
   screen.
7. **UX-AC7 (no dead ends)**: No error or empty state in any of the 6
   surfaces requires the user to leave the current screen to discover what
   to do next; every empty/error state renders its own call-to-action or
   explanation inline.
8. **UX-AC8**: A rejected-signature spike (≥5 in the trailing hour) is
   visible on Surface 3 without requiring the user to have proactively
   checked — a banner appears automatically, not on-demand only.
9. **UX-AC9**: Secrets (webhook HMAC secret, callback URLs) are never
   redisplayed in plaintext after initial creation, in any of the 6
   surfaces, verified by inspecting the network response payload of every
   `Get*`/`List*` RPC these surfaces call.

### Accessibility

10. **UX-AC10**: Every trigger-type-specific field group in the builder
    (Surface 2) is a `<fieldset>` with a `<legend>` (or `aria-labelledby`
    equivalent), and switching the `type` radio triggers an `aria-live`
    announcement of which fields are now shown — verified with a
    screen-reader (VoiceOver/NVDA) walkthrough of type-switching.
11. **UX-AC11**: All five `TriggerFireOutcome` states on Surface 3, and the
    enabled/disabled state on Surface 1, are distinguishable by a text label
    or icon+text pairing, not color alone — verified by viewing the panel
    with a grayscale filter/color-blindness simulator and confirming every
    status remains distinguishable.
12. **UX-AC12**: Every icon-only control (⋯ menu, [×] close, delete) across
    all 6 surfaces has an `aria-label`; every modal/dialog (Surface 4 test
    modal, delete-confirmation) is focus-trapped and closes on `Escape`.
13. **UX-AC13**: Sortable table headers on Surface 1/Surface 3 are
    keyboard-operable (`<button>`-in-`<th>` or `aria-sort`, not a bare
    `onClick` on a `<th>`) — this explicitly fixes forward, rather than
    replicates, the pre-existing gap research/ux.md §3 flagged in
    `ApprovalRulesPanel.tsx`'s `thSortable` pattern.
14. **UX-AC14**: Text/background color pairs used for the five status badges
    (fired/failed/rejected/no-match/disabled) meet WCAG AA contrast (≥4.5:1
    for text), using only tokens already defined in
    `web-app/src/app/globals.css` (`--success`/`--success-bg`,
    `--error`/`--error-bg`, `--warning`/`--warning-bg`, `--text-muted`,
    `--text-disabled`) per `.claude/rules/css-architecture.md` — no new hex
    values introduced. `fired_failed` (amber/warning) and `rejected` (red/
    error) must use visibly distinct token pairs from each other, not two
    shades of the same token, since research/ux.md §4 requires them to read
    as categorically different states.
15. **UX-AC15**: All `aria-live="polite"` announcements introduced on these
    surfaces (trigger enabled/disabled, test result, save success/failure,
    type-switch) are verified present via the same visually-hidden-span
    pattern as `ApprovalRulesPanel.tsx:366-372` — checked by inspecting the
    DOM for the live region's updated text content after each triggering
    action, not just visually confirming a toast appeared.

### Mobile

16. **UX-AC16**: Every interactive control introduced on these 6 surfaces
    (toggle, ⋯ menu, FAB, modal buttons, card row) has a touch target ≥44×44px
    at mobile viewport widths, matching this repo's existing convention
    (`docs/tasks/mobile-ux-improvements.md:296-297,396-397`).
17. **UX-AC17**: No surface relies on `:hover` alone to reveal an
    affordance (e.g. a row action only visible on mouse hover) — anything
    revealed on hover on desktop is always visible or reachable via a
    tap/focus-visible affordance on mobile/keyboard, per this repo's
    standing mobile+desktop dual-support convention
    (`feedback_mobile_desktop_ux.md`).
18. **UX-AC18**: Surface 1's list and Surface 3's execution history render
    as stacked cards (not a horizontally-scrolling table) at mobile
    viewport widths, since both have ≥5 logical columns — consistent with
    this repo's existing avoidance of horizontal table scroll on small
    screens.
19. **UX-AC19**: The Surface 1 "Add Trigger" action is reachable via a
    bottom-corner FAB on mobile, and secondary/low-frequency actions ("Test
    All") are hidden on mobile rather than crammed into a scrolling toolbar
    — directly mirroring `ApprovalRulesPanel.tsx`'s `mobileAddFab` +
    `headerButtonsHiddenOnMobile` split.

---

## Open UX questions carried forward (not blocking this doc, flagged for
implementation)

- Whether `TestTrigger` (Surface 4) requires a persisted `workflow_id` or can
  operate on an in-progress, unsaved form's field values — resolves
  UX-AC3's exact step count; implementer should confirm the RPC's actual
  signature (Task 7.2.1d) and adjust the "Save the trigger first" disabled
  state accordingly if it turns out to require persistence.
- Whether a fuller outbound-callback delivery history (beyond the
  interactive "Send Test Ping") is worth adding in a later pass — flagged in
  Surface 5 as a known gap, not designed here since plan.md's Observability
  Plan scopes delivery failures to logs only for this pass.
- Exact wording/threshold for the "rejected-count spike" banner (Surface 3,
  UX-AC8) — 5/hour is a placeholder default here; should be validated against
  real webhook traffic patterns once the feature ships, not treated as a
  final tuned value.
