# UX Design: session-pr-creation

Source inputs: `project_plans/session-pr-creation/requirements.md`,
`project_plans/session-pr-creation/research/ux.md`,
`project_plans/session-pr-creation/implementation/plan.md` (Phase 2, Epics
2.1-2.5). This document turns the plan's already-decided component/field/state
design into concrete wireframes, flows, and testable UX acceptance criteria.
It does not re-litigate decisions already made in the plan (extracted
component, two-RPC CQS split, in-flight guard, etc.) — see plan.md's Pattern
Decisions table for those.

---

## Surfaces designed

1. Trigger button — `SessionActionsOverflow.tsx` overflow menu item
2. Trigger button — `ReviewQueuePanel.tsx` "🔀 Create PR" button (same modal, second entry point)
3. `CreatePullRequestModal` — drafting/loading state
4. `CreatePullRequestModal` — editable form state
5. `CreatePullRequestModal` — existing-PR / "View PR" state
6. `CreatePullRequestModal` — submitting state
7. `CreatePullRequestModal` — success state (created vs. reused, with persist-failure sub-state)
8. `CreatePullRequestModal` — error state (draft-fetch failure, submit failure)

8 surfaces total (2 trigger contexts sharing one modal implementation, 6 modal states).

---

## Surface 1 & 2: Trigger button (three states, two entry points)

Per plan.md Epic 2.3/2.4, both `SessionActionsOverflow.tsx`'s overflow item
and `ReviewQueuePanel.tsx`'s button render the same three states and open the
same `CreatePullRequestModal`. Wireframe shows the overflow-menu context;
`ReviewQueuePanel`'s inline button follows the identical state machine.

```
State A — no PR yet, commits ahead of base (enabled)
┌───────────────────────────────┐
│ ⋮ Session actions              │
├───────────────────────────────┤
│  Restart                       │
│  Checkpoint                    │
│  🔀 Create PR                  │  ← data-testid="create-pr-trigger-<id>"
│  Delete                        │     onClick → open modal, fetch draft
└───────────────────────────────┘

State B — no PR yet, ZERO commits ahead of base (disabled)
┌───────────────────────────────┐
│  🔀 Create PR   (grayed out)   │  ← disabled, title="No commits ahead
│                                 │     of main yet"
└───────────────────────────────┘

State C — PR already exists (link, not a button)
┌───────────────────────────────┐
│  ✅ View PR #512                │  ← <a href=pr.htmlUrl target=_blank>
│                                 │     aria-label="PR #512: <title>"
│                                 │     data-testid="github-pr-link"
│                                 │     does NOT open the modal
└───────────────────────────────┘
```

### Interaction flow

1. User opens the session card's overflow menu (or is on `ReviewQueuePanel`).
2. State is derived from persisted fields only — `session.githubPrUrl` (State
   C) and the pre-fetched/known commits-ahead signal (State B) — never from
   local component state that could go stale across remount/navigation (per
   plan.md Task 2.3.1a, which explicitly removes `isRunningOneShot`/
   `oneShotResult` local state in favor of deriving from `session.githubPrUrl`).
3. **State A** click → opens `CreatePullRequestModal` with `isOpen=true`,
   triggers the draft fetch (Surface 3).
4. **State B**: button is `disabled`, has a native `title` tooltip
   ("No commits ahead of main yet"); clicking does nothing (browsers suppress
   click on `disabled` elements) — the gate is enforced by disabling the
   trigger, not by opening-then-failing.
5. **State C** click → navigates to GitHub in a new tab (`target="_blank"`);
   the modal is never opened for a session with an existing PR from this
   button — this is the one exception mentioned in ux.md research where the
   old "reopen to a view-PR affordance" idea is simplified: the plan's
   `DraftPullRequestResponse.existingPrUrl` still backs a defense-in-depth
   check inside the modal (Surface 5) for the race where a PR appears
   between page load and click, but the primary path from this button is a
   direct link, not a modal reopen.

### Edge case: commits-ahead becomes stale between page load and click

If the session gains its first commit after the page loaded (State B → A
transition happens server-side), the button only updates on the next
session-list refresh/poll — this is an existing, accepted staleness window
shared by every other derived-state button in this file (e.g. the restart
button's stopped/running state) and is not a regression this feature
introduces.

---

## Surface 3: Modal — drafting/loading state

```
┌──────────────────────────────────────────────────────────┐
│  Create Pull Request                                    ✕ │
│  ────────────────────────────────────────────────────── │
│                                                            │
│              ⏳  Drafting PR description…                 │
│                                                            │
│         (title/body/base-branch fields not yet shown)     │
│                                                            │
│                                          [ Cancel ]        │
└──────────────────────────────────────────────────────────┘
role="dialog" aria-modal="true" aria-labelledby="createPrDialogTitle"
```

### Interaction flow

1. Modal opens (`isOpen` flips true) → `useEffect` fires `draftPullRequest(sessionId)`,
   `isDrafting = true`.
2. While `isDrafting`, the form fields are not rendered (nothing to edit yet)
   — only a loading indicator and a `Cancel` button (still functional; the
   user can back out of a slow draft without waiting).
3. Focus moves into the dialog per `useFocusTrap` on open; there's no
   autofocus target yet since the title input doesn't exist until the draft
   resolves — focus lands on the dialog container itself (or `Cancel` as the
   only interactive element) until the effect resolves and remounts the form
   (Surface 4), at which point focus moves to the title input per plan.md
   Task 2.1.1d.
4. On response:
   - `existingPrUrl` non-empty → transition to Surface 5 ("View PR" state).
   - Otherwise → populate `title`/`body`/`baseBranch` from the response,
     transition to Surface 4.
   - `null`/thrown error → transition to Surface 8 (error state), draft
     fetch failure variant.

### Timing

No explicit timeout is specified in the plan for `DraftPullRequest` itself
(unlike `CreatePR`'s documented ~60s ceiling) — the loading state has no
artificial minimum or maximum display duration; it is purely a function of
RPC latency (LLM draft call + git diff computation).

---

## Surface 4: Modal — editable form state

```
┌──────────────────────────────────────────────────────────────┐
│  Create Pull Request                                        ✕ │
│  ────────────────────────────────────────────────────────── │
│  feature/rate-limit-toggle → main                              │  ← static
│                                                                   context text (ux.md §5
│                                                                   social-job recommendation)
│  Title                                                          │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ Add rate limit toggle                                   │    │  ← autofocus,
│  └───────────────────────────────────────────────────────┘    │     data-testid=
│                                                                   "create-pr-title-input"
│  Description                                                    │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ ## Summary                                               │    │  ← resizable,
│  │ Adds a per-user rate limit toggle to the settings page. │    │     min-height ~8 lines,
│  │                                                           │    │     data-testid=
│  │ ## Changes                                               │    │     "create-pr-body-input"
│  │ - New `RateLimitToggle` component                       │    │
│  │ - Wired to `/api/settings`                               │    │
│  │                                                     ▨    │    │  ← resize handle
│  └───────────────────────────────────────────────────────┘    │
│                                                                   │
│  Base branch                                                    │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ main                                                     │    │  ← data-testid=
│  └───────────────────────────────────────────────────────┘    │     "create-pr-base-branch-select"
│                                                                   │
│                                    [ Cancel ]   [ Create PR ]  │  ← submit disabled
└──────────────────────────────────────────────────────────────┘     if !title.trim()
```

Each field has a real `<label htmlFor="...">` (not just adjacent text) per
ux.md §3/§1's explicit callout on the program-picker dialog's a11y gap —
this is a hard requirement, not a nice-to-have, since three fields in one
dialog make ambiguous-role Playwright locators a real failure mode.

### Interaction flow

1. User edits any of the three fields freely — no field is read-only, no
   character limit enforced client-side (the backend/`gh` layer is the
   validation authority; per plan.md's Pattern Decisions, `baseBranch` is a
   plain string with no client-side format validation — `gh pr create --base`
   validates server-side and returns its own error).
2. `Create PR` submit button is disabled whenever `isSubmitting || isDrafting
   || !title.trim()` — body and base-branch have no required-non-empty gate
   (an empty PR body is a valid GitHub PR; an empty base branch falls back to
   `gh`'s own default-branch resolution per Task 1.1.1a).
3. `Cancel` closes the modal, discarding all edits (no "are you sure" —
   nothing has been created yet, so there is no destructive-action
   confirmation needed here, consistent with every other cancel-only dialog
   in `SessionActionsOverflow.tsx`).
4. `Escape` key closes the modal identically to `Cancel` (existing dialog
   contract, ux.md §1).
5. Backdrop click closes the modal identically to `Cancel`.
6. Clicking `Create PR` → transition to Surface 6 (submitting state).

---

## Surface 5: Modal — existing-PR / "View PR" state

Triggered when `DraftPullRequestResponse.existingPrUrl` is non-empty on the
initial draft fetch (a race: the trigger button's State C check passed
without a PR, but one was created between page load and the click — e.g. two
browser tabs, or the backlog automation path created one concurrently).

```
┌──────────────────────────────────────────────────────────┐
│  Create Pull Request                                    ✕ │
│  ────────────────────────────────────────────────────── │
│                                                            │
│   This session already has a pull request.                │
│                                                            │
│   → View PR #512                                          │  ← <a target=_blank>
│                                                              aria-label="PR #512"
│                                                              data-testid=
│                                                              "github-pr-link"
│                                                            │
│                                              [ Close ]     │
└──────────────────────────────────────────────────────────┘
```

### Interaction flow

1. No editable form is rendered — this is a dead end by design (the
   mechanical create path must never fire against a session that already has
   a PR, per AC4).
2. `View PR #512` opens the existing PR in a new tab; `Close` dismisses the
   modal without any RPC call.
3. This state is unreachable via a stale cache: it's derived directly from
   the just-fetched `DraftPullRequestResponse`, not from the page's
   possibly-stale `session.githubPrUrl` — so even the trigger-button race
   above self-corrects the moment the modal opens.

---

## Surface 6: Modal — submitting state

```
┌──────────────────────────────────────────────────────────────┐
│  Create Pull Request                                        ✕ │
│  ────────────────────────────────────────────────────────── │
│  feature/rate-limit-toggle → main                              │
│                                                                   │
│  Title                                                           │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ Add rate limit toggle                          (locked) │    │  ← disabled
│  └───────────────────────────────────────────────────────┘    │
│  Description                                                    │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ ...                                             (locked) │    │  ← disabled
│  └───────────────────────────────────────────────────────┘    │
│  Base branch                                                    │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ main                                            (locked) │    │  ← disabled
│  └───────────────────────────────────────────────────────┘    │
│                                                                   │
│                                  [ Cancel ]   [ Creating PR… ]  │  ← both disabled
└──────────────────────────────────────────────────────────────┘
```

### Interaction flow

1. All three inputs AND both buttons are disabled (not just the submit
   button) — per plan.md Task 2.1.1d / ux.md §3's "stale-value resubmit
   race" rationale: `CreatePullRequest` can take up to ~60s (push + `gh pr
   create`), and leaving inputs editable mid-flight invites a user to change
   the title and hit Enter again, or to close-and-reopen with half-submitted
   state.
2. `Cancel` is disabled during submission — the user cannot abandon an
   in-flight request that may already be creating a PR server-side; they can
   only wait for it to resolve to success or error.
3. No progress bar/percentage — a single indeterminate state
   ("Creating PR…") is sufficient, matching every other long-running action
   in this file (checkpoint creation, restart).
4. On response, transitions to Surface 7 (success) or Surface 8 (error).

---

## Surface 7: Modal — success state

Two content variants share one shell: **created** vs. **reused** (`already_existed`), each with an optional **persist-failure** sub-banner.

```
Variant A — created, persisted cleanly
┌──────────────────────────────────────────────────────────┐
│  Create Pull Request                                    ✕ │
│  ────────────────────────────────────────────────────── │
│   ✅ Created PR #512                                       │
│   → github.com/tstapler/stapler-squad/pull/512             │  ← data-testid=
│                                                              "github-pr-link"
│                                              [ Close ]      │
└──────────────────────────────────────────────────────────┘

Variant B — reused an existing PR (already_existed = true)
┌──────────────────────────────────────────────────────────┐
│   ✅ Updated PR #512                                        │
│   → github.com/tstapler/stapler-squad/pull/512             │
│                                              [ Close ]      │
└──────────────────────────────────────────────────────────┘

Variant C — created, but persist failed (persisted = false)
┌──────────────────────────────────────────────────────────┐
│   ✅ Created PR #512                                        │
│   → github.com/tstapler/stapler-squad/pull/512             │
│  ┌───────────────────────────────────────────────────┐    │
│  │ ⚠ PR created but couldn't be saved to the session —  │    │  ← role="alert"
│  │   refresh to check.                                   │    │     NOT styled as
│  └───────────────────────────────────────────────────┘    │     a failure/red
│                                              [ Close ]      │     error — amber/
└──────────────────────────────────────────────────────────┘     warning tone
```

### Interaction flow

1. `title/prNumber` copy: "Created PR #N" when `alreadyExisted=false`,
   "Updated PR #N" when `alreadyExisted=true` (plan.md Task 2.1.1e).
2. The PR link is always shown and always correct (the PR is real on GitHub
   regardless of `persisted`) — `href={prUrl}`, opens in a new tab.
3. Variant C's warning banner is additive, not a replacement for the success
   message — per plan.md's explicit Pattern Decisions row: a persist failure
   must never read as "PR creation failed" (that would invite a duplicate-
   creating retry). It is `role="alert"` for screen-reader announcement but
   visually distinct (warning/amber, not error/red) from Surface 8's failure
   styling, so a sighted user doesn't misread it as the same severity as a
   real failure.
4. `Close` dismisses the modal; the session card's trigger button
   transitions to State C ("✅ View PR #512") on the next render, driven by
   the now-updated `session.githubPrUrl` (assuming `persisted=true`; if
   `persisted=false`, the card may still show the old State A/B momentarily
   until a refresh re-syncs — this is called out explicitly in the warning
   copy: "refresh to check").
5. No auto-close-on-success timer — the user must explicitly click `Close`
   (or Escape/backdrop) so they have time to read the PR link/warning; this
   matches every other confirm-style dialog in the file (no dialog in
   `SessionActionsOverflow.tsx` auto-dismisses).

---

## Surface 8: Modal — error state

Two variants: **draft-fetch failure** (Surface 3 → 8) and **submit failure**
(Surface 6 → 8). Both keep the modal open with any field values intact.

```
Variant A — draft fetch failed (generic copy; form never rendered)
┌──────────────────────────────────────────────────────────┐
│  Create Pull Request                                    ✕ │
│  ────────────────────────────────────────────────────── │
│  ┌───────────────────────────────────────────────────┐    │
│  │ ⚠ Couldn't load PR draft — try again.                │    │  ← role="alert"
│  └───────────────────────────────────────────────────┘    │     data-testid=
│                                                              "create-pr-error"
│                                       [ Close ]  [ Retry ] │
└──────────────────────────────────────────────────────────┘

Variant B — submit failed (specific backend error; fields preserved)
┌──────────────────────────────────────────────────────────────┐
│  Create Pull Request                                        ✕ │
│  ────────────────────────────────────────────────────────── │
│  feature/rate-limit-toggle → main                               │
│  Title                                                           │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ Add rate limit toggle                                   │    │  ← user's edits
│  └───────────────────────────────────────────────────────┘    │     still present,
│  Description                                                    │     re-enabled
│  ┌───────────────────────────────────────────────────────┐    │     (not locked)
│  │ ...                                                      │    │
│  └───────────────────────────────────────────────────────┘    │
│  Base branch: [ main ]                                          │
│  ┌───────────────────────────────────────────────────────┐    │
│  │ ⚠ GitHub CLI is not configured. Please run              │    │  ← role="alert"
│  │   'gh auth login' first.                                 │    │     data-testid=
│  └───────────────────────────────────────────────────────┘    │     "create-pr-error"
│                                    [ Cancel ]   [ Create PR ]  │  ← re-enabled,
└──────────────────────────────────────────────────────────────┘     retry in place
```

### Error → copy → recovery action mapping (maps directly to plan.md's Epic 1.4 acceptance criteria and ux.md §4)

| Backend condition (plan.md source) | Modal copy shown | Fields preserved? | Exit path |
|---|---|---|---|
| Draft fetch fails (`DraftPullRequest` errors or returns `null`) | "Couldn't load PR draft — try again." (generic — no field values exist yet to be specific about) | N/A — form was never populated | `Retry` re-fires the draft fetch; `Close`/Escape/backdrop closes the modal entirely |
| `gh` not authenticated (`checkGHCLI()` fails inside `CreatePR`, surfaced verbatim, Task 1.4.1c / AC6) | Exact backend string, e.g. `"GitHub CLI is not configured. Please run 'gh auth login' first"` | Yes — title/body/baseBranch untouched | Re-enable inputs + `Create PR` button; user can retry in place after fixing `gh` auth out-of-band, or `Cancel`/Escape/backdrop to abandon |
| Push rejected (non-fast-forward, permission) (Task 1.4.1b, AC6) | Exact push-error string from `wt.PushBranch()` | Yes | Same as above — retry in place |
| Generic RPC/transport failure (network, `connect.CodeInternal`, `prNumber == 0` case per Task 1.4.1c) | Exact `err.Error()` string returned by the RPC — never a generic "Something went wrong" wrapper (AC6 is explicit that the specific error must surface) | Yes | Same as above — retry in place |
| Concurrent call rejected (`connect.CodeAlreadyExists`, in-flight guard, Task 1.4.1a) | `"PR creation already in progress for this session"` | Yes | Same as above — retry in place once the in-flight call resolves; the guard clears via `defer` so a retry shortly after will succeed once the first call finishes |
| Persist failure after a successful create | **Not an error state** — this is Surface 7 Variant C (success + warning banner), not Surface 8. Listed here only to make the distinction explicit: this is the one "partial failure" case that must NOT route through the error-state UI, because the PR itself did succeed. | N/A | `Close` — the PR already exists, nothing to retry |

### No dead ends

Every error variant above offers at least one of: `Retry` (re-fires the
failed RPC), an in-place retry via the still-enabled `Create PR` button (no
separate "Retry" button needed — the same submit button serves this role
once inputs are unlocked), or `Close`/Escape/backdrop-click (always
available, abandons the flow without side effects). No error state leaves
the user with only a disabled UI and no way out.

---

## Full state diagram

```
                    ┌─────────────────┐
   trigger click ──►│  Surface 3       │
   (State A only)   │  Drafting…       │
                    └────────┬─────────┘
                             │ draftPullRequest() resolves
                 ┌───────────┼───────────────┐
                 │           │               │
         existingPrUrl   success,      error/null
         non-empty       no existing PR      │
                 │           │               │
                 ▼           ▼               ▼
        ┌─────────────┐ ┌──────────┐  ┌─────────────────┐
        │ Surface 5    │ │ Surface 4│  │ Surface 8 (A)    │
        │ View PR      │ │ Editable │  │ Draft-fetch error│
        │ (dead end,   │ │ form     │  │ Retry / Close    │
        │  Close only) │ └────┬─────┘  └────────┬─────────┘
        └─────────────┘      │ submit           │ Retry
                              ▼                  │
                       ┌─────────────┐           │
                       │ Surface 6    │◄──────────┘
                       │ Submitting   │
                       │ (all locked) │
                       └──────┬───────┘
                     ┌────────┼────────┐
                 success              error
                     │                 │
                     ▼                 ▼
             ┌───────────────┐ ┌──────────────────┐
             │ Surface 7      │ │ Surface 8 (B)     │
             │ Success        │ │ Submit error      │
             │ (+/- warning)  │ │ fields intact,    │
             │ Close only     │ │ retry in place    │
             └───────────────┘ │ or Cancel/Escape   │
                                └──────────┬─────────┘
                                           │ retry submit
                                           └──► back to Surface 6
```

---

## UX Acceptance Criteria

Each criterion below is independently testable by a human clicking through
the running app (or via the Jest/Playwright tests plan.md's Epics 3.1/3.2
already specify — cross-referenced where applicable).

### Task completion

1. **Happy path in ≤ 3 clicks/steps**: from a session card with commits
   ahead and no existing PR, a user can create a PR in 2 clicks — (1) open
   overflow menu → click "Create PR" (opens modal, auto-drafts), (2) click
   "Create PR" submit (no required edits) — plus optional review/edit time
   that doesn't count as a "step." (Maps to Surfaces 1→3→4→6→7.)
2. **Viewing an existing PR takes 1 click**: from a session card with a PR
   already set, clicking "✅ View PR #N" opens the PR on GitHub in a new tab
   — no modal, no intermediate confirmation. (Surface 1 State C.)
3. **Editing before submit requires no more clicks than not editing**:
   changing the title, body, or base branch is a single in-place edit in the
   already-open modal — it does not require leaving and re-entering a
   separate "edit mode."

### Error states

4. **Every error state shows the specific backend-provided message, not a
   generic fallback**, for: `gh` not authenticated, push rejected, generic
   RPC failure, and concurrent-call rejection. Verified by: opening the
   modal against a session in each induced failure condition and confirming
   the `data-testid="create-pr-error"` element's text matches the backend's
   literal error string (see the mapping table above). (Backs plan.md
   Task 1.5.1e / AC6.)
5. **The persist-failure case never displays as a failure.** Verified by:
   inducing a `SaveInstances` failure after a successful `CreatePR` and
   confirming the modal shows the success PR link plus a distinctly-styled
   (non-error) warning banner, not the error-state UI. (Backs plan.md Task
   3.1.1c.)
6. **No dead ends** — every error state (draft-fetch failure, submit
   failure) offers both a retry path (re-enabled submit button or explicit
   `Retry` button) and an unconditional exit (`Close`/Escape/backdrop-click),
   verified by reaching each error variant and confirming both actions are
   present and functional.
7. **Field values survive a submit failure.** Verified by: editing all three
   fields, inducing a submit error, and confirming the title/body/base-branch
   inputs still show the edited values (not the original draft) after the
   error renders. (Backs plan.md Task 3.1.1d, `CreatePullRequestModal_should_PreserveFieldValues_When_SubmitFails`.)
8. **No duplicate-PR risk from the UI.** Verified by: confirming the submit
   button and all inputs are `disabled` throughout Surface 6 (submitting),
   and that firing two rapid clicks on "Create PR" results in exactly one
   `createPullRequest` call (client-side) — the server-side in-flight guard
   (plan.md Task 1.4.1a) is a second line of defense, not the only one.

### Accessibility

9. **Keyboard-only operation**: a user can open the modal, tab through
   title → body → base branch → Cancel → Create PR in that order, edit each
   field, and submit — entirely without a mouse. Escape closes the dialog
   from any focus position inside it.
10. **Focus management**: on open, focus moves into the dialog (per
    `useFocusTrap`); once the draft loads, focus moves to the title input
    (not the submit button — ux.md §3's explicit rationale: the body needs
    review before submission is the natural next action). On close (any
    exit path), focus returns to the trigger button that opened the modal.
11. **Screen-reader labeling**: every input has a programmatically-associated
    `<label htmlFor>` (verified via accessible-name computation, e.g. axe or
    manual VoiceOver/NVDA pass) — not merely adjacent visual text. The dialog
    itself is announced via `aria-labelledby` pointing at the `<h3>` title.
    The error/warning banners are announced automatically on appearance via
    `role="alert"` without requiring the screen-reader user to navigate to
    them.
12. **Color contrast ≥ 4.5:1** for all text against its background in both
    the default and error/warning states, including the warning-banner text
    in Surface 7 Variant C and the error text in Surface 8 (verified via a
    contrast-checking tool, e.g. axe DevTools or the `ui-web-design-guidelines`
    skill's audit, against the actual theme tokens used — no ad hoc colors
    per `.claude/rules/css-architecture.md`).
13. **No information conveyed by color alone**: the success/warning/error
    distinction (Surface 7 vs. 8) is also conveyed by icon (✅/⚠) and text
    content ("Created"/"couldn't be saved"/error message), not by background
    color alone — relevant for color-blind users distinguishing the amber
    warning from the red error state.
14. **Disabled-state semantics**: the trigger button's State B (no commits
    ahead) uses a native `disabled` attribute plus a `title` tooltip
    explaining why — not merely a visual/CSS-only disabled look — so
    assistive tech correctly reports it as unavailable and explains the
    reason on hover/focus.

### Consistency with existing patterns (regression guard)

15. **z-index uses the token, not a magic number**: the modal's stacking
    context is set via `vars.zIndex.modal`, verified by grepping the new
    `.css.ts` file for `9999` (must return no matches) per
    `.claude/rules/css-architecture.md`.
16. **Single entry point per session card** (AC7): after this feature ships,
    there is exactly one PR-creation-related affordance per session
    card/queue row at any given time (Create PR button XOR View PR link,
    never both, never a second differently-behaved button) — verified by
    `grep -rn "onRunOneShot" web-app/src/` returning no matches (plan.md
    Task 2.5.1's own verification step) and by visual inspection of both
    `SessionActionsOverflow.tsx` and `ReviewQueuePanel.tsx`.

---

## Notes for implementers (non-normative)

- The "`branch → base`" static context line in Surface 4 (e.g.
  `feature/rate-limit-toggle → main`) is a UX addition beyond what plan.md's
  tasks explicitly enumerate — it operationalizes ux.md §5's social-job
  recommendation ("showing who the PR will be attributed to / which
  repo+branch it targets"). It is a `<p>` of static text, not a new form
  field, and does not require a proto/RPC change: both `branchName` and
  `baseBranch` are already available client-side (`session.branch`/
  `session.branchName` from the session object, `baseBranch` from the
  `DraftPullRequestResponse`). Flag to the implementing engineer as a
  small, low-risk addition; if descoped for time, none of the acceptance
  criteria above depend on it.
- Surface 8 Variant A's `Retry` button is a UX-layer addition not explicitly
  named as a separate button in plan.md's task list (Task 2.1.1c only says
  "on a null/error response, set error to a generic message") — implementers
  should confirm whether "Retry" is a distinct button or whether closing and
  reopening the modal is the intended retry path. Recommendation: a distinct
  `Retry` button re-firing `draftPullRequest` is lower-friction and should be
  added; if not built, the fallback (`Close` then reopen the trigger) still
  satisfies "no dead ends" (criterion 6) since `Close` is always present.
