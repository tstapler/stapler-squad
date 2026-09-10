# UX Design: go-git-worktree-and-merge

**Phase**: SDD Phase 3 (design)
**Inputs**: `requirements.md`, `research/ux.md`, `decisions/ADR-002-two-feature-flags-and-merge-override-key.md`, `implementation/plan.md` (Epic 4: config flags → RPC service → settings UI)

## Scope

Per `research/ux.md`: this project has **no end-user-facing feature surface**.
The worktree/merge subsystem is invisible to anyone using a session; conflict
content stays terminal/LLM-agent-visible only, under the unchanged
always-abort-on-conflict contract. The only human-clickable surface is the
admin rollout panel already scoped in `requirements.md`'s Risk Control
section and locked down structurally by `implementation/plan.md`'s Epic 4.3
and ADR-002:

- **One** component, `NativeGitRolloutPanel` (not two panels) — plan.md
  Epic 4.3 already specifies a single component with two internal sections,
  mirroring `StreamHubRolloutPanel`/`TymuxRolloutPanel`'s established shape
  but consolidated because both flags share one `NativeGitRolloutService`
  RPC surface (`GetNativeGitRolloutStatus`,
  `SetNativeWorktreeGlobalOverride`, `SetNativeWorktreeSessionOverride`,
  `SetNativeMergeGlobalOverride`, `SetNativeMergeWorktreeOverride`).
- **No rollback-rehearsal UI** — plan.md's proto description explicitly
  omits rehearsal/env-var fields for this flag pair ("no legacy env var to
  deprecate, unlike stream_hub/tymux"), unlike both precedents. Do not carry
  that section over.
- **Different override keys per section** — ADR-002: the worktree section
  overrides key on **`sessionName`** (matches `TymuxRolloutPanel`'s pattern
  exactly); the merge section overrides key on **`worktreePath`** (no
  session-name lookup exists at that call site) — a genuinely new input
  shape not present in either precedent panel.
- **Both flags default OFF** (unlike `stream_hub`'s default-on) — this is
  corruption-blast-radius code per the Feasibility Risks section, so the
  panel's copy and badge language must say "default: off," not mirror
  Tymux's "default: off" only by coincidence of current rollout stage.

This document designs the one interactive surface (the panel) in full, and
gives condensed entries for the non-interactive surfaces the requirements
doc also names (logs, metrics/tracing, config schema, conflict-marker
output).

---

## 1. Interactive surface: `NativeGitRolloutPanel`

**Mounts**: `web-app/src/app/settings/features/page.tsx`, after
`<TymuxRolloutPanel />` (plan.md Story 4.3.2).
**Backing RPCs**: `NativeGitRolloutService` (5 RPCs, listed above).
**Reused styles**: `StreamHubRolloutPanel.css` classes, same as
`TymuxRolloutPanel` — no new stylesheet.

### 1.1 Wireframe

```
┌────────────────────────────────────────────────────────────────────────┐
│ Native Git Rollout                                                      │
│ Pure-Go worktree management and merge, replacing subprocess git calls.  │
│ Both flags default off. A change here takes effect on the next          │
│ operation — no restart. A session/path override below always wins       │
│ over the global setting in its own section.                             │
├──────────────────────────────────────────────────────────────────────── ┤
│ ⚠ Couldn't load rollout status — controls below may be stale. [Retry]   │  ← only on load failure
├────────────────────────────────────────────────────────────────────────┤
│ Native Worktree Management                                              │
│                                                                          │
│  Global override      [● Unknown — reload failed]                       │  ← badge: Unknown(amber) |
│    [Force on for everything] [Force off for everything] [Clear override]│    Not set(gray, default) |
│                                                                          │    Forced on/off(green/red)
│  Per-session canary overrides                                           │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │ backlog-fix-142                 [Forced on]           [Remove] │    │
│  │ pr-review-88                    [Forced off]          [Remove] │    │
│  └────────────────────────────────────────────────────────────────┘    │
│  (No sessions are currently overridden.)  ← shown instead when empty    │
│                                                                          │
│  [ Session title (existing or new)...      ▾ ] [Force worktree on for session] │
├────────────────────────────────────────────────────────────────────────┤
│ Native Merge                                                            │
│                                                                          │
│  Global override      [Not set (default: off)]                          │
│    [Force on for everything] [Force off for everything] [Clear override]│
│                                                                          │
│  Per-worktree-path overrides                                            │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │ /home/ty/.stapler-squad/worktrees/backlog-fix-142               │    │
│  │                                  [Forced off]         [Remove] │    │
│  └────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  [ Worktree path...                        ▾ ] [Force merge on for path]│
└────────────────────────────────────────────────────────────────────────┘
```

Both sections share one visual template: a status row (badge + 3 buttons),
an override list (or empty-state line), and an add-row (input + datalist +
button). This is the same template `StreamHubRolloutPanel`/
`TymuxRolloutPanel` already use for their single section — here it's
rendered twice with different labels, keys, and RPC calls, not two
components with duplicated markup.

### 1.2 Interaction flow

**Load** (on mount):
1. Panel renders `Loading…` (matches precedent's `if (loading) return <p>…`).
2. `GetNativeGitRolloutStatus` fires. On success: both sections populate
   their global-override badge and override lists from the response; the
   worktree section's session-title datalist and the merge section's
   worktree-path datalist populate from a concurrent `listSessions({})`
   call (session `title` → worktree section; session `path` → merge
   section — `Session.path` is "path to workspace repository root," which
   for a session's isolated git worktree is that worktree's own root).
3. On failure: see §1.3 Load failure — this is the one place this design
   deliberately diverges from the precedent (see note below).

**Toggle the global override** (either section):
1. Operator clicks "Force on for everything" / "Force off for everything" /
   "Clear override." Button disables (in-flight state, `busy=true`) but the
   *other* two buttons in the same row and the whole other section stay
   interactive — a stuck worktree RPC must never block clicking the merge
   section's emergency-off, and vice versa (two independent `busy` flags,
   one per section, not one panel-wide flag).
2. `SetNative{Worktree,Merge}GlobalOverride` fires with `ForceNative:
   true|false|undefined`.
3. On success: badge updates from the response (not optimistically) —
   "Forced on" / "Forced off" / "Not set (default: off)."
4. On failure: badge is left unchanged (showing the *last known-good*
   state, not a guessed new one); an inline error appears (see §1.3).

**Add a per-session / per-path override**:
1. Operator types (or picks from the datalist) a session title / worktree
   path. "Force … on for session/path" stays disabled while the field is
   empty or whitespace-only.
2. Click → `Set*SessionOverride` / `Set*WorktreeOverride` with the forcing
   value `true` (matches precedent: the add-row's only affordance is
   "force on"; forcing an entry to *off* is done by first adding it forced
   on, which is nonsensical — see §1.4 Gap carried over from precedent,
   flagged not silently copied).
3. On success: the new row appears in the list, the input clears.
4. On failure: input is **not** cleared (so the operator doesn't retype it)
   and an inline error appears.

**Remove an override**:
1. Click "Remove" on a row → `Set*Override` with no forcing value (clears
   the map entry), same call shape the precedent uses for removal.
2. On success: row disappears.
3. On failure: row stays, inline error appears.

### 1.3 Error and edge-case handling

| Situation | What the operator sees | Exit path |
|---|---|---|
| **Load failure** (`GetNativeGitRolloutStatus` errors) | Banner: "Couldn't load rollout status — controls below may be stale." with a Retry button. **Both sections' global-override badges show "Unknown — reload failed" (amber)**, not "Not set (default: off)." All four toggle/clear buttons in both sections are disabled until a reload succeeds; the override-add/remove controls stay enabled (their own RPCs are independent and still worth trying). | Click Retry → re-runs `GetNativeGitRolloutStatus`; success clears the banner and unlocks the toggle buttons. |
| **Emergency global kill while a conflicting override exists** — e.g. native worktree is corrupting sessions; operator clicks "Force off for everything" in the Worktree section, but a session in the per-session list is still `Forced on`. | After the global call succeeds, the panel appends a second, distinct warning (not the generic error banner — a `role="status"` note under the override list, not `role="alert"`, since nothing failed): "1 session override still forces native worktree management on for this session, which takes precedence over the global setting above: backlog-fix-142. Remove it below if this is part of the incident." Each named session's "Remove" button is highlighted (existing `removeButton` style, no new visual language). | Click Remove on the named row, or dismiss the note (it's informational, not blocking) — either way nothing else on the page is blocked. |
| **Mutation failure** (any `Set*` RPC — global, session, or path) | Inline error text scoped to that section (`role="alert"`), specific to the action: "Failed to update the global override — try again" for a toggle; "Failed to set session override — try again" / "Failed to set worktree-path override — try again" for add; "Failed to clear override — try again" for remove. Never a bare "Something went wrong." | The same button remains clickable immediately (no cooldown) — retry is the exit path. State shown is always the last confirmed server value, never a guess. |
| **`listSessions` fails** (datalist population only) | No visible error — datalists are a convenience, not a dependency (matches precedent's own `.catch(() => {/* non-fatal */})`). Both text inputs keep working as free text. | N/A — nothing to exit from. |
| **Operator adds a worktree path with no matching live session** (e.g. an archived session, or a path typed by hand for a one-off incident) | Accepted without client-side validation — the server doesn't require the path to belong to a currently-listed session (ADR-002 explicitly allows overriding a specific troublesome worktree that may already be gone). Row appears keyed by the exact string typed. | If typed wrong, "Remove" clears it — same exit path as any other row. |
| **Both sections mid-request at once** (operator fires worktree and merge actions back to back) | Each section has its own `busy` flag; a merge-section click never waits on a worktree-section RPC in flight, and vice versa. | N/A — no blocking. |

### 1.4 Gap carried over from precedent, flagged not silently copied

Like `StreamHubRolloutPanel`/`TymuxRolloutPanel`, the add-row can only force
an entry **on**; there is no UI affordance to add an override that forces a
session/path **off** except by first forcing it on with the opposite intent
inverted at the RPC layer (the RPCs do accept `forceNative: false` — the UI
just never sends it from the add-row). This is an existing product decision
in both precedent panels, not something introduced here, so this design
keeps it for consistency rather than inventing new interaction shape the
precedent doesn't have. If a future revision of any of these three panels
adds a per-row on/off choice, apply it to all three together so they don't
drift apart.

---

## 2. Non-interactive surfaces (condensed)

### 2.1 Structured logs (`slog`)

Representative line (per `implementation/plan.md`'s Observability section):

```json
{"level":"WARN","msg":"native git operation retried after contention","operation":"worktree.add","sessionName":"backlog-fix-142","implementation":"native","outcome":"retried"}
```

Acceptance criteria:
- Every native worktree/merge call logs `operation`, `sessionName` or
  `worktreePath` (whichever the call is keyed on), `implementation`
  (`"native"`/`"legacy"`), and `outcome`.
- A retry from the Ground-Truth Re-Query loop (plan.md Epic 2.5) logs at
  WARN, not ERROR — it's a contention signal, not a failure, and must not
  page anyone or fail a log-level-based alert.
- Log fields are greppable by `docs/how-to/debug-with-logs.md`'s existing
  pattern-clustering tool without a new field-name convention.

### 2.2 OTel tracing spans / metrics

Representative span name: `git.worktree.add`, `git.merge.main`, with
attributes `implementation`, `sessionName-or-worktreePath`, `outcome`.

Acceptance criteria:
- Span names and attributes match `docs/how-to/enable-opentelemetry.md`'s
  existing naming convention exactly (dot-separated, lowercase) — a regression
  is discoverable in Tempo the same way as the prior diff-timeout
  investigation, with no new dashboard-building step.
- A conflict-rate counter distinguishes `no_changes` / `fast_forward` /
  `three_way_merged` / `conflicted` outcomes, per `requirements.md`'s
  Observability Requirements — an operator can see a merge-correctness
  regression (e.g. a spike in `conflicted`) without reading logs.

### 2.3 `config.json` flag schema

Representative fragment (new fields only):

```json
{
  "feature_flags": { "native_git_worktree": false, "native_git_merge": false },
  "native_worktree_session_overrides": { "backlog-fix-142": true },
  "native_merge_worktree_overrides": { "/home/ty/.stapler-squad/worktrees/backlog-fix-142": false }
}
```

Acceptance criteria:
- Field names match `implementation/plan.md`'s Domain Glossary exactly
  (`NativeWorktreeFeatureFlag = "native_git_worktree"`,
  `NativeMergeFeatureFlag = "native_git_merge"`) — no drift between the Go
  struct tags and the panel's RPC field names.
- A hand-edited `config.json` (no restart) is picked up on the next call,
  matching every other flag in this file today — this project introduces no
  new caching behavior.
- Absent keys default to `off` for both flags (verified by
  `EffectiveNativeWorktreeEnabled`/`EffectiveNativeMergeEnabled`'s existing
  test coverage per the plan, not a new UI concern).

### 2.4 On-disk conflict-marker output (terminal/LLM-agent-visible)

Not a UI screen, but explicitly named in `requirements.md`'s Rabbit Holes as
a real consumer surface (backlog automation's fix-agent prompts show
conflict markers to an LLM). Representative sample:

```
<<<<<<< HEAD
our version of the line
=======
their version of the line
>>>>>>> theirs
```

Acceptance criteria:
- Byte-for-byte identical to real git's default (`MergeStyleDefault`, 7-char
  markers, no `|||||||` ancestor section) — verified by differential test
  against real `git merge` output, not just "some" conflict text.
- `git status --porcelain` and `git diff` run against the same worktree
  after a native conflict report both show the file the same way they would
  after a real `git merge` conflict — this is what makes the existing
  `VcsWidgetFileList.tsx` "Conflict — resolve before merging" UI activate
  correctly for the rare case (out of this project's default contract,
  which still aborts) that a future caller stops auto-aborting.
- No new web-app work is triggered by this surface today — confirmed by
  `research/ux.md`'s grep showing zero references to `ConflictedFiles` in
  `web-app/src`.

---

## 3. UX acceptance criteria

**Task completion**
1. An operator can force the native worktree path off globally in **1
   click** from the Features page (button is visible without scrolling past
   the existing two panels' typical height) once the panel is open.
2. An operator can add a per-session or per-worktree-path override in
   **2 steps**: type/pick a value, click the section's "Force … on" button.
3. An operator can fully reverse any override (global or per-item) in
   **1 click** ("Clear override" / "Remove").

**Error states**
4. Every failed mutation (`Set*` RPC) shows a specific, action-named error
   message (never a bare "Something went wrong") and leaves the affected
   control immediately retriable with no cooldown.
5. A load failure shows a distinct "Unknown — reload failed" badge state
   (not a silent fallback to "Not set (default: off)") and disables only
   the four global-override buttons, not the whole panel.
6. **No dead ends**: every error state (load failure, mutation failure) has
   a visible action that can resolve it (Retry button, or the same control
   simply being clickable again) — none require a page reload.

**Precedence clarity**
7. When a session/path override conflicts with the section's global
   setting after an emergency global toggle, the panel names the
   conflicting override(s) and offers Remove inline — an operator does not
   have to cross-reference the override list manually to find what's still
   forcing the old behavior.

**Consistency with precedent**
8. The panel's visual language (badge colors, button labels, `data-testid`
   naming scheme) matches `StreamHubRolloutPanel`/`TymuxRolloutPanel`
   exactly except where ADR-002 forces a difference (merge section's
   worktree-path key instead of session name; no rollback-rehearsal
   section for either flag).

**Accessibility**
9. Every interactive control (toggle buttons, remove buttons, text inputs)
   is reachable via Tab in visual order and operable with Enter/Space —
   no custom keyboard trap.
10. Every icon-only or ambiguous control has an `aria-label` (e.g. "Force
    native worktree management off for all sessions," not just "Force
    off," when the accessible name would otherwise be ambiguous between
    the two sections' identical button text).
11. Error banners use `role="alert"`; the precedence-conflict note (§1.3)
    uses `role="status"` so screen readers announce it without the
    urgency implied by `alert` (nothing failed).
12. Badge and button text meets ≥ 4.5:1 contrast in both light and dark
    theme, reusing `StreamHubRolloutPanel.css`'s already-audited
    `badgeEnabled`/`badgeDisabled` tokens — no new color introduced for the
    "Unknown — reload failed" state; use the existing warning/amber token
    already in the design system rather than a one-off hex value.

**Non-interactive surfaces**
13. Log lines, span names, and config field names use the exact identifiers
    in `implementation/plan.md`'s Domain Glossary — verifiable by grep, not
    just style review.
14. Conflict-marker output is verified byte-identical to real git's by an
    automated differential test, not manual inspection.
