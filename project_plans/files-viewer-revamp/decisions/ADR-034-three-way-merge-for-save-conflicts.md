# ADR-034: 3-way merge for save conflicts

**Status**: Proposed (2026-10-16); library choice pending Task 2.3.2a
**Plan**: `implementation/plan.md` Epic 2.3; amended by ADR-036 (Story 2.3.3)

## Context
Owner decided (2026-10-16) that a real 3-way merge is in scope. `@codemirror/merge` 6.12.2 provides two-way `MergeView`/`unifiedMergeView` only. Agents and the user both edit the same files, so overwrite/reload alone force one side to lose work.

## Decision
- Inputs: `MergeBase = Available(text) | Unavailable`; `Available` is the LF-normalized content the buffer was opened from, held in memory for editable <= 1 MB buffers. `mine` = current doc text; `theirs` = `Conflict.FullText.content` from `WriteFile` (no extra round trip). Merge is offered only when `MergeBase` is `Available` **and** the conflict carries `FullText`; for `Unavailable` (either side) the flow is two-way and `merge3` is never called. A restored draft has `Available` only if its token still equals disk (see ADR-032).
- Merge computation uses a vetted diff3 library behind `merge3(base, mine, theirs): MergeResult`; do not hand-write a diff algorithm. Candidate: `node-diff3`; Task 2.3.2a records license, size, maintenance and may substitute. Loaded via dynamic import.
- UI: `@codemirror/merge` shows result-vs-theirs; conflict regions get Mine/Theirs/Both controls; Save stays disabled until none remain; save uses `expected_token = theirs token`. Unified presentation on mobile with 44px hunk actions. All three inputs are already LF-normalized (client load function; server `FullText`); `merge3` neither normalizes nor re-applies EOL or BOM. The server re-applies them from the token-verified disk file on save (ADR-031), so a BOM/CRLF file keeps both after a merge.
- Review step (pre-mortem #9): Merge never saves. `MergeResult.autoMergedHunks[]` marks each auto-merged hunk `nearTheirs` when it overlaps or lies within 1 line of a theirs hunk; the result pane labels hunk origins, and when any hunk is `nearTheirs` Save stays disabled until the user ticks "Review merged result". diff3 merges adjacent but semantically coupled edits cleanly, so a clean merge is not proof of correct code.
- Correctness bar: property tests (`merge3(b,t,b)=t`, `merge3(b,b,t)=t`), the chosen examples in Story 2.3.2, never auto-resolve a true conflict.
- **ADR-036 amendment (2026-10-16)**: **documents are merged, deltas are not rebased.** For T0 the flow above is unchanged (the client's `MergeBase`, `merge3` over LF documents, `theirs = FullText`); a pending ChangeSet is never transformed against the other side's change, so there is no operational transformation. For T1 (files over 1 MiB) `theirs = Unavailable{TooLarge}` and Merge and Overwrite are disabled; the choices are Keep editing, Reload (the pending ChangeSet is kept as a draft) and **same-position rebase**: `PreflightFilePatch(new_base_token, edits)` verifies that each edit's `deleted_crc32c` and the CRC of its full touched lines still hold at the same line numbers in the new file; if all hold the client adopts the new token and re-sends, if lines moved it **fails closed** (no diff-and-guess rebase exists in the code). The merge UI is mounted through the `MergeSurface` port (`CmMergeSurface` wraps `@codemirror/merge` and `merge3`); an engine with `features.mergeView=false` hides Merge and the 2-way flow still ships. Plan: Story 2.3.3.


## Consequences
Extra memory for `MergeBase` (bounded by the 1 MB editable cap; T0 only, since ADR-036's T1 files have no `MergeBase` and use the same-position rebase); a new dependency pair (`@codemirror/merge`, diff3 lib), both lazy. Triad repair: the phone merge view has its own global kill switch `files:merge-phone` (default on) and a runtime degrade to the 2-way conflict flow when the flag is off, the container is < 360px or the library fails to load (ux.md section 10); desktop merge is unaffected. The owner dogfoods the phone merge (plan Task 2.5.4a) before the `files:editing` flip. Merge events are recorded as `files.edit.merge_open` / `merge_saved` (ADR-035).

## Alternatives rejected
Overwrite/reload/keep-editing only (loses one side); hand-rolled diff3 (silent-wrong risk per build-vs-buy); server-side merge (needs base on the server, no benefit).
