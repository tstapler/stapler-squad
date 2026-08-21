# UX Research: session-pr-creation

Agent 5 (UX) — SDD research phase.

## 1. Comparable UX patterns already in this codebase

### Modal pattern to reuse: `SessionActionsOverflow.tsx`

This file already implements 5 distinct dialogs (restart confirm, delete
confirm, checkpoint create, autonomous-mode confirm, steer, program picker),
all following one consistent shape. The new "Create PR" modal should be a
sixth instance of this exact pattern, not a new one:

- **Portal + backdrop click-to-close**: `createPortal(<div className={confirmDialog} onClick={...close...}><div ...onClick={stopPropagation}>...</div></div>, document.body)`. Backdrop click closes; click inside the dialog does not. See [SessionActionsOverflow.tsx:288-314](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L288-L314) (restart) and [:355-399](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L355-L399) (checkpoint — closest structural precedent: title text input + submit/cancel + inline error).
- **Dialog a11y contract**: `role="dialog" aria-modal="true" aria-labelledby="<id>DialogTitle"`, with a `<h3 id="...">` matching. `useFocusTrap(dialogRef, isOpen, triggerRef)` traps focus and returns it to the trigger button on close ([:154-158](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L154-L158), hook at `web-app/src/lib/hooks/useFocusTrap.ts:17`).
- **Escape-to-close**: `onKeyDown={(e) => { if (e.key === "Escape") setIsOpen(false); }}` on the dialog div.
- **Loading state**: a single `isSubmitting`-style boolean disables the confirm button and swaps its label (`"Saving..."`, `"Restarting..."`, `"Creating PR…"`). Cancel is also disabled while in flight so the user can't abandon a request that already started.
- **Error display**: a single `errorMessage`-classed `<p>`/`<span>` under the input, cleared on reopen/cancel, set from `err instanceof Error ? err.message : "<generic fallback>"` in the `catch` block. Errors are always inline in the dialog, never a toast-only surface, for actions with an editable-before-submit step.
- **Select input precedent**: the "Change Program" dialog ([:786-812](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L786-L812)) is the only existing dialog with a `<select>` — directly relevant since this modal needs a base-branch selector. Note it does *not* wire `aria-label`/`htmlFor` on the select (relies on adjacent paragraph text) — the e2e test for it (`tests/e2e/session-program-change.spec.ts:26-28`) locates it via `page.getByRole('dialog', {name}).getByRole('combobox')`, i.e. it works today only because there's a single combobox in the dialog. **Do not copy this gap** — the new modal has three inputs (title, body, base branch), so each needs a real `<label htmlFor>` or `aria-label` to be locator-safe per `.claude/rules/e2e-test-conventions.md`.
- **z-index**: existing dialogs hardcode `zIndex: 9999` inline (program picker, [:780](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L780)) rather than using the token ladder. `.claude/rules/css-architecture.md` explicitly forbids new hardcoded z-index — the new modal should use `vars.zIndex.modal` (1000) from `web-app/src/styles/theme-contract.css.ts:202`, not copy the 9999 precedent.
- **Trigger button state machine**: the existing "Create PR" overflow item already encodes a 4-state label cycle — `"Create PR"` → `"Creating PR…"` (disabled) → `"✅ PR Created"` / `"❌ Retry?"` ([:609-618](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L609-L618)). This is the button the new modal replaces in place (per AC7) — reuse its label states, but "✅ PR Created" should become a clickable link to the PR (see §4) rather than a dead label, and clicking it while a PR already exists should reopen to a "view PR" affordance, not attempt re-creation.

### "Ship PR" precedent (`BacklogItemDetail.tsx`) — a *contrasting* precedent, not a template

`BacklogItemDetail.shipPR.test.tsx` documents a **single-click, no-modal** flow: `case "ship_pr": await triggerShipPR(item.id); break;` ([BacklogItemDetail.tsx:561-563](/home/tstapler/Programming/stapler-squad/web-app/src/components/backlog/BacklogItemDetail.tsx#L561-L563)). There is no pre-fill/edit step — it fires directly, matching the *backlog* automation path that requirements.md explicitly says is out of scope to change. This confirms the new modal is a **genuinely new UI surface** (no existing edit-before-create modal to copy verbatim) — it borrows dialog *chrome* from `SessionActionsOverflow.tsx` but its *content* (title input, body textarea, base-branch select) has no in-repo precedent to lift wholesale.

Do not mimic the Ship-PR button's zero-review immediacy for the new modal — AC1/AC2 explicitly require a review step before the mechanical create fires, which is the entire point of this feature (see JTBD emotional job, §5).

### PR display precedent: `GitHubPRsSection.tsx`

`web-app/src/components/unfinished/GitHubPRsSection.tsx:76-83` shows the convention for a PR link: `<a href={pr.htmlUrl} target="_blank" rel="noopener noreferrer" aria-label={\`PR #${pr.number}: ${pr.title}\`}>`, with `data-testid="github-pr-card"`. Reuse this `aria-label` shape (`PR #<n>: <title>`) for the session-card PR link this feature adds/keeps.

## 2. User mental model — comparison to GitHub Desktop / VS Code

Both GitHub Desktop's "Create Pull Request" and VS Code's GitHub Pull Requests extension share a flow the acceptance criteria already mirror closely:

| Step | GitHub Desktop / VS Code | This feature (per requirements.md AC1-2) |
|---|---|---|
| Trigger | Button visible once there's a pushed/pushable branch with commits ahead of base | "Create PR" action, gated on active worktree + ≥1 commit ahead of base |
| Title prefill | Last commit subject (single commit) or branch name (multi-commit) | Session title |
| Body prefill | Commit body / PR template | `headless.DraftPRDescription` output (LLM-generated from diff) |
| Base branch | Dropdown, defaults to repo default branch, shows ahead/behind count | Dropdown, defaults to repo main branch (AC1, AC3) |
| Edit before submit | All fields editable inline | Title, body, base branch editable (AC2) |
| Submit | "Create Pull Request" button, opens PR, returns URL | Mechanical `GitWorktree.CreatePR` call (AC3) |

One mental-model gap worth flagging to the plan phase (not a blocker, but a likely reviewer question): GitHub Desktop shows an **ahead/behind commit diff summary** in the same dialog so the user can sanity-check *which* commits will be in the PR before submitting — this repo's diff viewer already exists elsewhere on the session card, so the modal doesn't need to duplicate it, but the modal copy should make clear it's summarizing "the session's current diff" so users don't expect to re-select commits.

Base-branch dropdown source: requirements.md doesn't specify how the branch list is populated (all repo branches vs. just the default + current tracking branch). `gh pr create` itself only needs a single string; recommend keeping this to a plain text input or a small fixed list (default branch + explicit override) rather than fetching the full branch list — fetching all branches for a dropdown is unscoped work not mentioned in the requirements and risks scope creep the plan phase should explicitly bound.

## 3. Accessibility — concrete ARIA contract for the new modal

Following the `SessionActionsOverflow.tsx` dialog contract (§1) and `.claude/rules/css-architecture.md` / `.claude/rules/e2e-test-conventions.md`:

```tsx
<div role="dialog" aria-modal="true" aria-labelledby="createPrDialogTitle" ref={dialogRef}>
  <h3 id="createPrDialogTitle">Create Pull Request</h3>

  <label htmlFor="createPrTitle">Title</label>
  <input id="createPrTitle" type="text" data-testid="create-pr-title-input" ... />

  <label htmlFor="createPrBody">Description</label>
  <textarea id="createPrBody" data-testid="create-pr-body-input" ... />

  <label htmlFor="createPrBaseBranch">Base branch</label>
  <select id="createPrBaseBranch" data-testid="create-pr-base-branch-select" ... />

  {error && <p role="alert" className={errorMessage} data-testid="create-pr-error">{error}</p>}

  <button data-testid="create-pr-submit" disabled={isSubmitting || !title.trim()}>
    {isSubmitting ? "Creating PR…" : "Create PR"}
  </button>
  <button data-testid="create-pr-cancel" disabled={isSubmitting}>Cancel</button>
</div>
```

Points not covered by copy-pasting the checkpoint/program-picker dialogs verbatim:

- **Every input needs its own `<label htmlFor>`** (see §1's callout on the program-picker gap) — three inputs in one dialog means ambiguous-combobox failures in Playwright's `getByRole` locators if labels are skipped.
- **`role="alert"` on the error message** — none of the existing dialogs' `errorMessage` elements have this; add it here since screen-reader users need the failure surfaced without re-focusing (AC6 requires *specific* error text, which is wasted if it's not announced).
- **Body textarea sizing** — LLM-generated PR bodies (`DraftPRDescription`) run several paragraphs; a single-line-height textarea (matching the checkpoint dialog's `renameInput` styling) would truncate visually. Needs a taller, resizable textarea, not the existing `renameInput` class as-is.
- **Focus order on open**: existing dialogs `autoFocus` the single input (checkpoint label, program select). With three fields, autofocus the title input (first, most commonly edited) — do not autofocus the submit button given the body needs review first (this directly serves the "confidence before it's public" emotional job, §5).
- **Loading-state disables all three inputs**, not just the submit button — the checkpoint dialog only disables its submit/cancel buttons, but since this is a longer-running RPC (git push + `gh pr create`, up to 60s per `CreatePR`'s own timeout) leaving inputs editable mid-submit invites a stale-value resubmit race.

## 4. Error states

Map directly from `GitWorktree.CreatePR` ([session/git/worktree_git.go:329-392](/home/tstapler/Programming/stapler-squad/session/git/worktree_git.go#L329-L392)) and its `checkGHCLI`/`findExistingPR` dependencies to modal-visible copy — AC6 requires the *specific* error, not a generic failure state:

| Backend condition | Where it surfaces | Modal UX |
|---|---|---|
| No commits ahead of base | Should be checked **before** opening the modal (AC1 gates the action itself on "at least one commit ahead") | Don't open the modal at all — the trigger button should be disabled/hidden with a tooltip/title (`title="No commits ahead of <base> yet"`), matching the existing disabled-button-with-title pattern already used for other gated actions in this file (e.g. `title="Session stopped — restart to resume working"`, [:542](/home/tstapler/Programming/stapler-squad/web-app/src/components/sessions/SessionActionsOverflow.tsx#L542)) |
| `gh` not authenticated (`checkGHCLI()` fails) | Surfaces as the RPC error string on submit | Inline error in the dialog (`role="alert"`), keep the dialog open with the user's edits intact (title/body/base-branch should not be lost on failure — re-submit after fixing auth should not require retyping) |
| **PR already exists** (`findExistingPR` finds one) | `CreatePR` returns the *existing* URL/number successfully — this is **not an error path** at the RPC layer | Per AC4, this must not read as a failure. If the modal is opened for a session that already has a PR, skip the create flow entirely — show a "View PR" link/state instead of the create form (mirrors the existing "✅ PR Created" button-state precedent, §1). If a race occurs mid-submit (PR created between the pre-check and the click), the success path should still land the user on "PR created" copy, not silently no-op |
| Push rejected (e.g. non-fast-forward, permission) / network/RPC failure | Generic `err.Error()` string from the RPC, or a ConnectRPC transport error | Inline error, same as above — don't paper over it with a generic "Something went wrong"; requirements.md AC6 is explicit about surfacing the *specific* error text returned |

General pattern: **all error states keep the modal open with field values intact** — none of the failure modes above should force the user to re-enter title/body/base-branch, since re-typing an LLM-generated body after a transient `gh` auth hiccup is exactly the kind of friction this feature exists to remove.

## 5. Jobs-to-be-done

- **Functional job**: "Get my finished session's changes in front of a human reviewer as a real GitHub PR, without hand-writing a title/description or manually running `git push && gh pr create` in the worktree." The modal exists to make the *mechanical* path (already built for backlog sessions) reachable for the manual dashboard flow — see requirements.md's core gap #2.
- **Emotional job**: **confidence the PR looks right before it becomes public** — this is the load-bearing reason the modal exists at all instead of keeping the one-click `RunOneShot` agent flow. An LLM-drafted title/body is a *first draft*, not a final artifact; the review-and-edit step (AC1/AC2) is what converts "an agent published something under my name" anxiety into "I checked it and it's fine." This should shape modal copy: don't word the submit button "Publish" or "Ship" (implies irreversible/already-public framing that undercuts the pre-submit review moment) — "Create PR" (matching GitHub's own verb) keeps the mental model that the human is the one taking the action, with the LLM output as an editable starting point, not an autonomous act.
- **Social job**: this modal is the **explicit handoff point from agent-driven work to human review** (requirements.md's "Why This Matters" framing, referenced in the task prompt). The moment of clicking "Create PR" is when a solo/session-scoped changeset becomes a team-visible artifact with the user's name on it as the PR author. This argues for showing *who* the PR will be attributed to / which repo+branch it targets somewhere in the modal (even briefly, e.g. "`feature/foo` → `main`" as static text near the base-branch selector) so the handoff moment isn't just "trust the defaults."

## Summary of concrete recommendations for the plan phase

1. Build the modal as a new dialog block inside `SessionActionsOverflow.tsx` (or extracted to its own component file if the existing file's 882 lines are already a maintainability concern — plan phase should decide), reusing `confirmDialog`/`dialogContent`/`dialogActions`/`cancelButton`/`submitButton`/`errorMessage` CSS classes and the `useFocusTrap` hook, not inventing new modal chrome.
2. Replace the existing `onRunOneShot`/"Create PR" overflow-menu button's click handler to open this modal instead of firing `RunOneShot` directly (AC7 — single entry point). Keep its 4-state label cycle, but wire "✅ PR Created" to open/link the existing PR rather than being a dead label.
3. Every field needs `<label htmlFor>` + `data-testid`; error text needs `role="alert"`; use `vars.zIndex.modal` not a hardcoded value.
4. Gate the trigger itself (not just the submit) on "≥1 commit ahead of base" — don't rely on the modal's submit-time error for that specific case, since it's cheaply checkable in advance and produces a better UX (disabled button + tooltip vs. open-then-fail).
5. If the session already has a PR, the action should present a "View PR" state, not reopen the create form (AC4/AC5 convergence).
