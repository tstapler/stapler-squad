# Architecture Research — omnibar-creation-stuck-modal

## Scope note

Prior architecture research touching this component (`project_plans/omni-bar-quick-navigation/research/architecture.md`,
`project_plans/omni-bar-session-search/research/architecture.md`, `project_plans/bulk-select-ux/research/architecture.md`)
covers detector priority ordering, search UX, and bulk-select state — none analyze `handleSubmit`'s branch
structure or the `onClose` dismissal mechanism. There is no directly reusable prior analysis for this bug;
this is fresh ground.

## 1. Is this a simple CRUD-style bug fix?

**Yes.** This is UI state-management (a React `useState` boolean not being reset on every code path), not
multi-actor business logic. No EventStorming table, no domain events, no aggregate boundaries involved.
Confirmed no EventStorming needed.

## 2. Tracing `onClose` end to end

`Omnibar.tsx:50` — prop signature `onClose: () => void`.

**Origin (`web-app/src/lib/contexts/OmnibarContext.tsx`):**
```ts
const [isOpen, setIsOpen] = useState(false);
...
const close = useCallback(() => {
  setIsOpen(false);
  setInitialInput(undefined);
  setInitialTitle(undefined);
}, []);
...
<Omnibar isOpen={isOpen} onClose={close} ... />   // line ~275-277, UNCONDITIONAL render
```

**Consumption (`Omnibar.tsx:1203`):**
```ts
if (!isOpen) return null;
```

Two facts fall out of this that matter for the bug:

1. **`Omnibar` is rendered unconditionally by its parent** — `OmnibarProvider` does not do
   `{isOpen && <Omnibar .../>}`. The gating is entirely internal (`return null`). This means the
   `Omnibar` function component **instance is never unmounted** across open/close cycles — all of its
   `useState` hooks, including `isSubmitting` (`Omnibar.tsx:205`), persist in memory for the lifetime of
   the app, not just for one "open" session.
2. There is a dedicated **"reset state when closed" effect** (`Omnibar.tsx:588-599`) that exists
   specifically to compensate for fact #1 — it fires whenever `isOpen` flips to `false` and clears
   `input`, `detection`, `formState`, `uiState`, `error`, and dispatches `reset_to_discovery`. **It does
   not reset `isSubmitting`.** This is the concrete, verifiable gap: the one piece of state whose staleness
   causes user-visible harm (a permanently disabled "Creating…" button) is the one piece this effect
   forgot.

```ts
// Omnibar.tsx:587-599
useEffect(() => {
  if (!isOpen) {
    setInput("");
    setDetection(null);
    setFormState(INITIAL_FORM_STATE);
    setUIState({ showAdvanced: false, dropdownIndex: -1, dropdownDismissed: false, resultHighlightIndex: -1, atSuggestIndex: -1 });
    setError(null);
    lastSuggestedNameRef.current = "";
    prevDetectionTypeRef.current = null;
    dispatchMode({ kind: "reset_to_discovery" });
  }
}, [isOpen, dispatchMode]);
```

### The real mechanism (evidence-based, not speculative)

Given React 19 (`web-app/package.json:83`, `"react": "^19.0.0"`) automatically batches state updates
everywhere — including inside resolved-promise continuations — a plain `setIsOpen(false)` call is not
architecturally likely to be silently "lost" by React itself under normal operation. Static reading of this
code does not show a code path where `onClose()`'s own `setIsOpen(false)` call is skipped or overridden
(no competing `setIsOpen(true)` fires in the same tick; no memoization boundary blocks the prop from
reaching `Omnibar`).

What the code **does** show, and what best explains the reported symptom without requiring an
unfalsifiable React-internals theory, is a **compound failure built from two true architectural facts**:

- **Fact A (verified above):** `Omnibar` never unmounts; `isSubmitting` is excluded from the
  close-reset effect.
- **Fact B (verified above):** the two broken branches (SpawnShell ~1003-1027, Alias ~1038-1071) only
  call `setIsSubmitting(false)` inside `catch`. On the happy path they call `onClose()` and return,
  *trusting* that closing the modal makes `isSubmitting`'s value moot.

That trust is misplaced given Fact A. Even in the "working" case, `isSubmitting` is left `true` forever in
component memory once `onClose()` is called successfully on the happy path — it is simply invisible
because the component currently renders `null`. The stuck-modal symptom becomes *visible* the next time
`isOpen` flips back to `true` for that same long-lived component instance (reopening the omnibar via
keyboard shortcut, `openInCreationMode()`, etc.): the freshly reopened modal immediately renders in the
disabled "Creating…" state because `isSubmitting` was never cleared from the prior submission — with no
new submit in flight. This is indistinguishable, from the user's perspective, from "the modal never closed."

A secondary, narrower failure mode also fits the report without contradicting the above: any exception
raised **between** the `await onCreateSession(...)` resolving and the `onClose()` call on the happy path
(e.g. `addRecentShellCommand(shellCommand)` at `Omnibar.tsx:1020`, which touches `localStorage` and can
throw under Safari private-browsing/mobile quota restrictions) is still inside the `try` block, so it
*is* caught and *does* reset `isSubmitting` via the existing `catch` — but it also means `onClose()` on
line 1021 is skipped entirely (it comes after the throwing line), leaving the modal open with an error
shown instead of "Creating…". This is a related-but-distinct bug (the ordering of `onClose()` relative to
best-effort side calls like `addRecentShellCommand`) worth flagging but not the one under investigation
here, since the user report describes a stuck spinner, not a stuck-open form with an error.

**Bottom line:** the fix must address both symptoms of the same root cause — inconsistent
try/catch/finally across branches (proximate cause, matches the bug summary exactly) — and should also
close the gap in the close-reset effect (defense-in-depth against the same class of "state assumed moot
because the modal is closed" bug recurring elsewhere), since the component's never-unmounts design makes
that effect the only real safety net against a persisting `isSubmitting`.

## 3. Recommended code structure

**Minimal fix, no new abstraction.** Two concrete changes:

1. Add `try { ... } finally { setIsSubmitting(false); }` to the SpawnShell branch (~1003-1027) and the
   Alias branch (~1038-1071), matching the pattern already correctly used in the default branch
   (~1073-1169). This is a mechanical copy of an existing, proven pattern already present three times
   in this same function (default branch, plus the same shape again inside the nested
   `try`/`catch` at line 1142-1157) — it is not a new idiom for this codebase.
2. Add `setIsSubmitting(false)` to the "reset state when closed" effect (`Omnibar.tsx:588-599`), which is
   a one-line addition to an existing effect that already exists for exactly this purpose (resetting
   stale state when `isOpen` goes false) — not a new mechanism.

**Explicitly reject a shared `submitAndClose` wrapper/hook.** Reasons:

- Agent 1's stack research found no existing shared "async submit" hook anywhere in this codebase, and
  every comparable component (per that research) hand-rolls its own submit/loading state. Introducing
  one shared abstraction here, for a bug fix, would be inventing a pattern with a single call site inside
  this same function — a textbook case of the repo's own
  `.claude/rules/interface-pollution-checklist.md` smell #1 ("speculative interface/abstraction with no
  near-term second consumer").
- The three branches are only *near*-identical: SpawnShell builds `sessionData` differently, has a side
  effect (`addRecentShellCommand`) the others don't, and the default branch has extra logic (path-not-found
  recovery flow at 1142-1157, `saveHistory` at 1160-1162) that doesn't fit a generic wrapper cleanly
  without parameterizing away most of the behavior that makes each branch distinct. Forcing a shared
  helper here trades a 4-line mechanical duplication for a wrapper with several optional callback
  parameters — worse for readability, not better.
- This matches the repo's stated ponytail/YAGNI convention of minimal diffs: fix the two branches that are
  actually broken, in place, using the pattern the third branch already demonstrates is correct. Do not
  refactor the correct branch or extract shared structure as part of a bug-fix diff.

If a fourth near-identical branch is added in the future, that is the appropriate trigger point to
reconsider extraction (per the interface-pollution checklist's own guidance: extract once a second real
need exists, not preemptively).

## Files and line references

- `web-app/src/components/sessions/Omnibar.tsx:205` — `isSubmitting` state declaration
- `web-app/src/components/sessions/Omnibar.tsx:1003-1027` — SpawnShell branch (needs `finally`)
- `web-app/src/components/sessions/Omnibar.tsx:1038-1071` — Alias branch (needs `finally`)
- `web-app/src/components/sessions/Omnibar.tsx:1073-1169` — default branch (correct reference pattern)
- `web-app/src/components/sessions/Omnibar.tsx:588-599` — close-reset effect (needs `isSubmitting` added)
- `web-app/src/components/sessions/Omnibar.tsx:1203` — `if (!isOpen) return null;` (component never unmounts)
- `web-app/src/lib/contexts/OmnibarContext.tsx:148-152` — `close()` definition
- `web-app/src/lib/contexts/OmnibarContext.tsx:275-277` — unconditional `<Omnibar>` render (parent never unmounts it)
- `web-app/package.json:83` — `"react": "^19.0.0"` (automatic batching, rules out a simple "lost setState" theory)
