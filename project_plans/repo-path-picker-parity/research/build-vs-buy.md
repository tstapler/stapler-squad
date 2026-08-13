# Build vs Buy: Repo Path Picker Parity

## 1. Combobox/autocomplete OSS library — not justified

`web-app/package.json` has **no** combobox/autocomplete dependency today: no `downshift`,
`cmdk`, `react-aria`/`@react-aria/combobox`, `@headlessui/react`, or similar. The only
relevant UI primitives present are Radix packages (`@radix-ui/react-accordion`, `-dialog`,
`-slot`, `-tabs`, `-tooltip`) — none of which is a combobox/listbox primitive — plus
`fuse.js` (fuzzy search, used elsewhere, not a UI widget) and `react-arborist`/`react-virtuoso`
(tree/virtualized list, unrelated).

`RepoPathInput` (`web-app/src/components/ui/RepoPathInput.tsx`) is a working, in-house
combobox already consumed by 4 call sites (`BacklogItemForm`, `NewShellDialog`,
`WorkflowForm`, `LocalFileBrowser`) via `useSessionRepoPaths()` + `usePathCompletions()` +
`PathCompletionDropdown`. It already handles: open/close state, arrow-key navigation,
Enter-to-select, Escape-to-close, click-outside dismissal, ARIA (`aria-autocomplete`,
`aria-controls`, `aria-activedescendant`), and a `role="listbox"` dropdown
(`PathCompletionDropdown`) with history-vs-filesystem entry styling.

This task is a **swap of 2 plain `<input>` elements for an already-widely-used internal
component**, not new combobox functionality. Introducing `downshift`/`cmdk`/`react-aria`
combobox here would mean maintaining two parallel autocomplete implementations
side-by-side (the new library-based one for these 2 fields, the hand-rolled one for the
other 4 existing consumers) — directly contradicting the requirements doc's own explicit
finding: *"It is the correct reuse target — no new picker component should be built."*
There is no functional gap (keyboard nav, ARIA, filtering, history entries are all already
implemented) that would justify the dependency, bundle-size, and API-surface-learning cost
of pulling in a new library for 2 call sites when an equivalent, already-integrated
component exists. **Verdict: build (reuse existing in-house component), not buy.**

## 2. SaaS/managed API — not applicable

This is entirely client-side UI state (form fields backed by Redux + local component
state); no network service, hosted data, or third-party managed capability is involved.
N/A, no further evaluation needed.

## 3. Redux selector sort-with-tiebreak — hand-written comparator is appropriate

Current code (`web-app/src/lib/store/sessionsSlice.ts`, `selectActiveSessionsSortedByUpdatedAt`):

```ts
export const selectActiveSessionsSortedByUpdatedAt = createSelector(
  selectAllSessions,
  (sessions) =>
    sessions
      .filter((s) => s.status !== SessionStatus.UNSPECIFIED)
      .sort((a, b) => Number(b.updatedAt?.seconds ?? 0) - Number(a.updatedAt?.seconds ?? 0))
);
```

`web-app/package.json` has **no `lodash`** (or `lodash-es`, `es-toolkit`, `remeda`, etc.)
as a dependency — so "use `sortBy` since it's already a dependency" doesn't apply; adding
lodash (even `lodash.sortby` as a single-function package) would be a **new dependency**
purely to express a 3-key descending sort with tiebreaks, which is more than the problem
warrants.

The needed fix (R3) is a small, fully-specified, well-understood comparator — sort by
`updatedAt.seconds` desc, then `createdAt.seconds` desc, then `id` ascending — expressible in
native `Array.prototype.sort` in ~5 lines with no ambiguity about semantics (unlike, say,
a stable multi-locale string collation, where reaching for `Intl.Collator` or a library
would be justified because subtle correctness bugs are easy to introduce by hand). A
native comparator:
- keeps `createSelector`'s memoization simple (no extra library call in the hot path),
- matches the existing codebase style (plain `.sort()` is what's already there),
- needs zero new dependency, install, or bundle weight for a comparator this small and
  self-contained.

**Verdict: hand-write the comparator.** Reaching for `lodash.sortBy` (or similar) would
add a new dependency for a task node's-own `Array.sort` already solves cleanly, and
`sortBy` doesn't natively express "descending, multi-key, with explicit numeric coercion
of possibly-undefined proto timestamp fields" any more clearly than a direct comparator
does — if anything a hand comparator is easier to read here since each tiebreak key needs
custom `?? 0` / numeric coercion handling that `sortBy` doesn't remove.

## 4. Fork or adapt — adapt is correct; no missing props found

Adapting/extending `RepoPathInput` in place (not forking a new component) is consistent
with the project's reuse goal and is what the requirements doc already concludes:
*"the correct reuse target — no new picker component should be built."*

Checked `RepoPathInput`'s current prop surface against both target call sites in
`OmnibarCreationPanel.tsx`:

```ts
interface RepoPathInputProps {
  id?: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  error?: string;
  placeholder?: string;
  required?: boolean;
  hint?: string;
  detectGitHubUrl?: boolean;
  onSelect?: (entry: CompletionEntry) => void;
  "data-testid"?: string;
}
```

- **Parent Directory field** (`OmnibarCreationPanel.tsx:488-501`): uses `id`,
  `placeholder`, `value`, `onChange`, plus an external `<label htmlFor>` and a separate
  `<span className={hint}>` for helper text. Every one of these is already covered by
  `RepoPathInput`'s prop surface (`id`, `placeholder`, `value`, `onChange`, and `hint` —
  though the current code renders the hint span itself outside the input, which the
  existing consumers pass through the `hint` prop instead; either approach works, this is
  a call-site styling choice, not a missing capability).
- **Existing Worktree Path fallback** (`OmnibarCreationPanel.tsx:651-660`): uses `id`,
  `placeholder`, `value`, `onChange` — identical shape, identical coverage. The
  conditionally-rendered `<select>` sibling (populated-worktrees case) is explicitly out
  of scope per the requirements doc and untouched by this component swap.

No prop gap exists. Neither field needs `detectGitHubUrl` (both are local directory
pickers, not GitHub-URL-accepting fields — `detectGitHubUrl` is opt-in and defaults off,
so leaving it unset is correct), and neither needs new props added to
`RepoPathInputProps`. The only actual code change required *inside* `RepoPathInput.tsx`
itself is R6 (Escape-key `stopPropagation`/`stopImmediatePropagation` on its own keydown
handler at `RepoPathInput.tsx:134-137`), which is a bug fix benefiting all 6 eventual
consumers (the existing 4 plus these 2), not new feature surface.

**Verdict: adapt `RepoPathInput` in place.** No fork, no new component, no prop-surface
extension needed beyond the R6 Escape fix already scoped in the requirements.
