# Build vs. Buy: modal-focus-trap

Requirements already decided to reuse the existing in-house `useFocusTrap` hook
(`web-app/src/lib/hooks/useFocusTrap.ts`) rather than adopt a new dependency. This
document validates that call rather than relitigating it.

## 1. Existing OSS options and why not adopting one now is reasonable

| Library | What it offers | Verdict for this fix |
|---|---|---|
| `focus-trap` / `focus-trap-react` | Mature, handles nested traps, `initialFocus`/`fallbackFocus` config, MutationObserver-based re-scan, escape-key hooks | Not justified — would be a net-new dependency plus a second focus-trap code path alongside the 5 existing `useFocusTrap` adopters, doubling the surface area to maintain for a one-time fix |
| `@radix-ui/react-dialog` (`FocusScope`) | Full dialog primitive: focus trap, portal, escape/outside-click dismiss, scroll lock | Already a dependency (see §2) but adopting it here means rewriting all 7 modals' markup/structure, which requirements explicitly rule out as out of scope |
| `react-aria` / `react-aria-components` | Most rigorous focus-management implementation (Adobe), handles virtual/portal focus edge cases | Not a dependency today; pulling it in for 7 call sites is disproportionate — it's designed to replace a whole component system, not patch a hook |
| `@headlessui/react` | Similar to Radix, includes `Dialog` with built-in trap | Not a dependency; same disproportionate-adoption argument as react-aria |

Given only 7 call sites need the fix, an in-house hook already proven in production at 5
other call sites, and requirements explicitly scoping out any migration, extending the
existing hook is the reasonable choice. Introducing a library now would mean maintaining
two parallel focus-trap implementations (old hook for 5 sites, new library for 7) with no
plan to reconcile them — worse than the status quo.

## 2. Dependency check — flag: `@radix-ui/react-dialog` is already installed

`web-app/package.json` (dependencies, confirmed by direct read) has:
```
"@radix-ui/react-dialog": "^1.1.15",
```
already used by `web-app/src/components/ui/Modal.tsx`, `components/backlog/BacklogTourModal.tsx`,
and `components/onboarding/OnboardingModal.tsx`. None of the 7 in-scope backlog modals
(`ReviewChangesModal`, `BacklogFileBrowserModal`, `VaguenessPromptModal`, `GateVerdictBox`,
`CommitPushModal`, `WorktreeDiffModal`, `BacklogQueueSection`) use it — all 7 are hand-rolled
`<div role="dialog" aria-modal="true">` markup (confirmed by grep across each file).

This does change the calculus slightly in the abstract (a battle-tested focus trap via
Radix's `FocusScope` is one prop away for any modal already built on `@radix-ui/react-dialog`),
but it doesn't change the recommendation for *this* fix: adopting it here would require
restructuring all 7 modals onto the Radix `Dialog` primitive, which is exactly the
migration requirements.md rules out. It's relevant context for a **future** dialog-system
consolidation (see §5), not for this bug fix.

Confirmed absent from both `dependencies` and `devDependencies`: `focus-trap`,
`focus-trap-react`, `react-aria`, `react-aria-components`, `@headlessui/react`.

## 3. LLM-generated hook vs. battle-tested library — correctness assessment

`useFocusTrap.ts` is ~64 lines. Current behavior:
- Computes focusable-element snapshot once per activation effect run (the known latent bug
  called out in requirements — doesn't re-scan on DOM mutation).
- Selector: `a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])`, filtered to exclude anything inside `[aria-hidden="true"]`.
- Tab/Shift+Tab cycling via boundary check against `first`/`last` and a single global `keydown` listener.
- Returns focus to `triggerRef` on deactivation.

Gaps relative to mature libraries (focus-trap, Radix `FocusScope`):
- **No re-scan on mutation** — the bug this project is explicitly fixing at the two named call sites, and a real gap for any future modal with dynamically-appearing content (e.g., a diff modal that lazy-loads file content).
- **No shadow DOM traversal** — `querySelectorAll` doesn't pierce shadow roots. Not currently a risk: nothing in the 7 target modals uses shadow DOM (no web components in this codebase).
- **No nested-iframe handling** — same story, not applicable; no iframes in these modals.
- **No sentinel-div technique** — mature libraries insert invisible focus-catcher elements at the DOM boundaries to handle focus changes not caused by Tab (e.g. programmatic `.focus()` calls from elsewhere, or a mouse click outside the trap). The hook only intercepts the `Tab` key itself, so a click into background content or a screen-reader-driven focus move could still escape. This is a real, if narrower, gap versus focus-trap/Radix.
- **Radio groups**: arrow-key navigation within a radio group is native browser behavior and unaffected by this hook either way — not a distinguishing gap.

None of these gaps is triggered by the actual content of the 7 modals in scope (no shadow
DOM, no iframes, no radio groups identified in `ReviewChangesModal`, `BacklogFileBrowserModal`,
`VaguenessPromptModal`, `GateVerdictBox`, `CommitPushModal`, `WorktreeDiffModal`,
`BacklogQueueSection`). The one gap that *is* concretely in scope — stale focusable-element
snapshot — is small, well-understood, and cheap to fix in place: replace the one-time
`querySelectorAll` snapshot with either (a) a `MutationObserver` on the container, or (b)
computing `first`/`last` lazily inside `handleKeyDown` instead of once at effect-setup time.
Either fix is a localized, easily-reviewed change to an already-small function — the
correctness risk of extending this hook further is low and does not clear the bar for
pulling in a new dependency.

The sentinel-div / non-Tab-focus-escape gap is a legitimate but pre-existing limitation
shared by all 5 current adopters too; fixing it is out of scope for a bug labeled
specifically as "Tab/Shift+Tab moves focus out of the modal" and should be tracked
separately if it becomes a real complaint (no evidence in the backlog item that it has).

## 4. Prior art elsewhere in the monorepo

- Go binary/tmux app (`session/`, `server/`, root Go packages): no concept of DOM focus
  applies; `grep -rn "focus.trap\|FocusTrap" --include="*.go" .` returned nothing. No prior
  art to fork.
- No sibling directory under `~/.stapler-squad/` matching `*focus-trap*` or `*focustrap*`.
- The only other in-repo focus-management code is Radix's internal `FocusScope` (transitively
  bundled via `@radix-ui/react-dialog`), already covered in §1–2.

Nothing to fork; the existing `useFocusTrap.ts` is the sole and sufficient prior art.

## 5. Final recommendation

| Option | Verdict |
|---|---|
| (a) Keep extending the in-house hook | **Recommended.** Matches requirements' stated scope, has 5 proven adopters, the one relevant known gap (stale snapshot) is small and localized to fix, and none of the library-only capabilities (shadow DOM, iframes, sentinel divs) are needed by the 7 target modals. |
| (b) Adopt a library now | **Not recommended.** Would mean two parallel focus-trap implementations in the same codebase (hook for 5 sites, library for 7+), contradicts requirements' explicit scope exclusion, and none of the libraries' extra correctness guarantees are exercised by these modals' actual DOM structure. |
| (c) Defer library adoption to a future dialog-system rewrite | **Viable.** `@radix-ui/react-dialog` is already a dependency and already used by 3 components (`ui/Modal.tsx`, `BacklogTourModal.tsx`, `OnboardingModal.tsx`) that get focus trapping for free via Radix's `FocusScope`. A future consolidation of all modals (including the 7 backlog ones) onto that shared `Modal.tsx` primitive would eliminate the in-house hook entirely and is worth a separate backlog/ADR item — but is explicitly out of scope for this bug fix. |
