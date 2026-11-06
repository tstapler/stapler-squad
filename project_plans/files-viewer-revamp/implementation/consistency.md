# Cross-artifact consistency: files-viewer-revamp

Artifacts: `requirements.md`, `implementation/plan.md`, `design/ux.md`, `decisions/ADR-030..034`.
Date: 2026-10-16. Totals: **4 BLOCKER, 11 CONCERN, 9 NITPICK**.

**Phase 4 repair status (2026-10-16)**: B1-B4 RESOLVED; C1-C5, C7-C9 RESOLVED; C6, C10, C11 partly RESOLVED / ACCEPTED as follow-ups (see each); N1 RESOLVED, N2-N9 ACCEPTED (not fixed, none blocks implementation). Counts after repair, recounted by script: 12 epics / 38 stories / 88 tasks, effort rows 38, raw 7,660k tokens, 12,470k CU. Repair rows P4-* are in the plan's "Review repair log".

## Mechanical checks (VERIFIED by command)

| Check | Result |
|---|---|
| Epic / story / task counts (`grep -c` on `^## Epic`, `^#### Story`, `^##### Task`) | Originally 12 / 37 / 85. After the Phase 4 repair: 12 / 38 / 88 (+Story 1.3.6, Tasks 1.3.6a/b, 2.1.2f), matching header, effort-table footer and repair log |
| Effort table rows | Originally 37 rows / 85 tasks / 7,530k raw / 12,259k CU. After repair: 38 rows, 88 tasks, 7,660k raw, 12,470k CU ("7.66M / 12.47M"), verified by script (class weights, x0.93 x1.75 per row) |
| Duplicate task ids in plan | none |
| Tasks a-e in Story 2.1.2 | a proto, b FileWriter, c validation pipeline, d delegate/remote-access, e tests; every in-text reference ("2.1.2b-e", "Task 2.1.2d", "Task 2.1.2c") points at the right letter. No half-applied renumbering found |
| Cross-references to nonexistent stories/tasks | none, except the bare "Task 0.1.1" (see N1) |
| Stale `line_ending` / `has_bom` | only in `GetFileContentResponse` (Task 0.2.3a, Story 0.2.3 AC, read side, correct) and as explicit "removed" notes; none left in `WriteFileRequest` |
| Stale `info/exclude` | only in "dropped" notes (Story 2.1.1, Task 2.1.1a, ADR-031, repair rows) |
| Stale per-scope flag language | none outside "global-only" statements (requirements.md:78, plan Risk Control, Story 0.2.2, ADR-031) |
| Stale `WorktreeDir != ""` gate | none; only the "why not" note (Task 0.2.1b), ADR-031 and Story 2.1.2 AC pair, all agree on `ActiveDir != "" && ActiveDir == ExistingDir` |
| Duplicated ux ids | `U-EDIT-8` and `U-EDIT-9` each defined twice (ux.md:551-554) |

## BLOCKER

**B1. Paused-session search: plan searches the repo root, ux shows an empty state (plan vs ux, and ux vs itself).**
Plan Story 1.1.2 ("Root labelling"): a paused session whose worktree is gone is searched and returns `stats.root_is_fallback=true` so the UI says "searching repo root". ux.md S1 (section 3.4) has an empty state "Session paused; worktree unavailable" with a Resume action, and U-SEARCH-6 requires exactly that; the same table also has a "Search root fallback: Label 'Searching <root>'" row, so ux contradicts itself. Both cannot fire for the same input.
Resolution: pick one. If search falls back to the repo root (read side uses `ExistingDir`), change U-SEARCH-6 and the paused row to a banner plus results; otherwise make `SearchContent` refuse when `ActiveDir != ExistingDir` and add that AC to Story 1.1.2.
**Status: RESOLVED. Search refuses the repo-root fallback (`FailedPrecondition WorktreeMissing` when `ActiveDir != ExistingDir`); `root_is_fallback` dropped from `SearchStats`; ux root-fallback row removed; U-SEARCH-6 and ADR-030 aligned (plan Story 1.1.2, Task 1.1.2a, P4-B1)**

**B2. `WriteFile` refusal table is ambiguous for a truncated-read token (plan Story 2.1.2, internal).**
The code map says "`Truncated` -> `FailedPrecondition`" and "malformed token (incl. hash `-`) -> `InvalidArgument`". A truncated read always yields a hash-`-` token, and the Refusals AC lists "truncated-read token (hash `-`)" and requires "the specific error code and `ReadOnlyReason` from the table". Two codes are both "correct". The same ambiguity applies to ADR-031 Wire shape and to Task 2.1.2c ("parse token (reject `-`)") and Story 2.4.3.
Resolution: state which wins (suggest: hash `-` parses fine and is rejected as `FailedPrecondition Truncated`/`Binary`/`TooLarge` per the stored reason, `InvalidArgument` only for unparseable strings), and say how `ReadOnlyReason` is recovered when the token carries only `-`.
**Status: RESOLVED. Well-formed `-` token => `FailedPrecondition` (Truncated/Binary/TooLarge); only unparseable => `InvalidArgument` (glossary, Story 2.1.2, Task 2.1.2c, ADR-031, P4-B2)**

**B3. Duplicate ux acceptance-criterion ids `U-EDIT-8` and `U-EDIT-9` (ux.md:551-554).**
`U-EDIT-8` is both the vim escape guarantee and the stale-banner criterion; `U-EDIT-9` is both "dirty preview tab promoted" and "rejected save shows reason class". The ux header says ids are "stable for the validation phase", and the plan's repair log and ux 8.4 / risk 3 cite `U-EDIT-8` for the vim escape. A validation matrix keyed on id cannot be built; one criterion would silently go unmapped. Probably a half-applied insert (the later pair was the older numbering that should have shifted to 10-12; `U-EDIT-10` also exists).
Resolution: renumber to a unique sequence (U-EDIT-1..12) and update citations.
**Status: RESOLVED. Second pair renumbered U-EDIT-11 (stale banner) and U-EDIT-12 (rejected save); U-EDIT-8 (vim escape) and U-EDIT-9 (dirty preview) keep their ids, so the plan/ADR-033/ux 8.4 citations of U-EDIT-8 were already correct; validation.md mapping updated (P4-B3)**

**B4. Keyboard-resizable panes (U-A11Y-4) has no story, and the plan claims it already exists.**
Plan "Deferred / Out of Scope" says the panes "already resize by keyboard (UX U-A11Y-4)". ux U-A11Y-4 and 7.2 require `role="separator"` with `aria-valuenow` and arrow-key resize plus Ctrl+B. Reading `web-app/src/components/sessions/TreeResizeHandle.tsx` and `web-app/src/lib/hooks/useResizablePanel.ts` finds no `role`, `aria-*`, `Arrow*` or `onKeyDown` (grep returned nothing); `FilesTab.tsx` has a window keydown handler tied to `panel.collapsed` (lines 97-114) that was not checked in detail. Tech Debt table says `FileTree` is "extend as-is", so nothing implements it.
Resolution: add a task (likely under Story 1.3.5 or 1.5.1) for a keyboard-operable separator, and correct the Deferred paragraph.
**Status: RESOLVED. Confirmed again by reading the two files (pointer handlers only). Added Story 1.3.6 (Tasks 1.3.6a/b), effort row, Wave 4 placement; corrected the Deferred paragraph; `FilesTab.tsx` needs no edit because it spreads `panel.handleProps` (P4-B4)**

## CONCERN

**C1. Flag name has three spellings.** requirements.md `files_viewer_v2`/`files_editing` (plan header explains the rename to `files:viewer-v2`), but ux.md:471 uses `files:viewer_v2` (underscore) which matches neither. Resolution: use `files:viewer-v2` in ux.md; add the rename note to requirements.md.
**Status: RESOLVED. `files:viewer-v2` everywhere; requirements.md carries the rename note**

**C2. Write-log outcome vocabulary.** Plan Observability: `outcome = ok|conflict|denied|disabled|error`. requirements.md ("conflict-rejected"), ux.md:476 and U-OBS-1 use `conflict_rejected`. A log assertion test will fail one way. Resolution: align (suggest plan adopts `conflict_rejected` or ux/requirements adopt `conflict`).
**Status: RESOLVED. `conflict_rejected` adopted in plan Observability, requirements, ux U-OBS-1**

**C3. Directory-mode chip text differs.** ux.md:251 and plan Unresolved Questions: "Editing the repo root"; plan Story 2.5.2 AC: "Editing the main checkout". Resolution: one string (ux owns copy), reflected in a plan AC; currently no editing-story AC asserts the chip exists at all (only the 2.5.2 manual line).
**Status: RESOLVED. "Editing the main checkout" in ux, plan; Task 2.2.1c now owns an assertion for the chip**

**C4. InputProfile rule.** ADR-033: `HardwareKeyboard` when `(pointer: fine)` matches **or** a non-229 physical keydown is seen. Plan Story 2.4.1 AC: "fine pointer + first non-229 physical keydown" (reads as AND). A laptop with a touchscreen differs under the two readings. Resolution: use the ADR's OR in the AC.
**Status: RESOLVED. Story 2.4.1 AC uses the ADR-033 OR rule**

**C5. Tab limits disagree.** ux section 5: 30 open tabs, 31st closes the oldest clean non-pinned tab with an announcement or a toast with Close others. Plan Story 1.2.2 and ADR-032: cap 10 on mobile only, silent LRU eviction of clean buffers, no desktop cap, no toast. Resolution: decide the numbers and the user-visible announcement; add the missing AC.
**Status: RESOLVED. 10 mobile / 30 desktop, LRU clean eviction with polite announcement, all-dirty toast (Story 1.2.2 AC, ux section 5, ADR-032)**

**C6. Buffer states in ux have no plan representation.** ux section 2 lists `clean | dirty | stale | conflict | offline-dirty`, and S7 says Keep editing leaves the buffer "marked conflict: disk changed". Plan `Buffer.Loaded` carries only `dirty` (+ `kind`, tokens); stale is derived in a hook (Story 1.3.4, clean buffers only); conflict/offline-dirty have no field or reducer action. ux "Stale with unsaved edits: Changed on disk - review diff" banner has no plan story (1.3.4 is clean-only). Resolution: add the states (or say they are derived) in the glossary and a 1.3.4/2.3.1 AC for the dirty case.
**Status: ACCEPTED (partly resolved). Display states documented as derived in the glossary (no reducer fields). The "stale with unsaved edits -> Review diff" banner has no story; accepted follow-up to wire into Task 2.3.1b at implementation time**

**C7. `Connect ReadMaxBytes` has no owning change task.** Story 0.1.3 AC says "a task flags if it must be raised"; no task in Epic 2.1 raises it (Task 2.1.2d covers guard registration and remote-access tiers only). Requirements list "Connect max message size" as open. Resolution: add to Task 2.1.2d or 2.1.2b explicitly.
**Status: RESOLVED. Conditional Task 2.1.2f (XS) owns raising `ReadMaxBytes`; Story 0.1.3 AC points to it**

**C8. `FilesTab.tsx` edit-ordering list is incomplete.** Plan critical-path section orders 1.2.2c -> 1.2.2d -> 1.4.2b -> 1.5.1c -> 2.2.1c -> 2.2.2b, but Tasks 1.3.2a and 1.3.4b (and Story 1.3.2 Files) also edit `FilesTab.tsx`, and 1.3.x runs in Wave 4, before 1.4.2b (Wave 5). The "never in the same wave on different workers" rule is then not enforced for 1.3.2/1.3.4 (both Wave 4). Resolution: add them to the ordered list or route them through `useFilesDeepLink`/`useBufferFreshness` so `FilesTab.tsx` gets a one-line registration.
**Status: RESOLVED. 1.3.2a and 1.3.4b added to the `FilesTab.tsx` ordered list and touched-by list; same worker in Wave 4**

**C9. Mobile-reveal handle and edit-mode surfaces missing in tasks.** U-MOB-7 requires a visible reveal handle in landscape editing; Task 2.4.2b only says "landscape hides top/bottom bars". Resolution: add the handle to 2.4.2b's text and an AC.
**Status: RESOLVED. Reveal handle added to Task 2.4.2b with a jest assertion**

**C10. Command palette scope.** ux S12 defines `>` commands, `:` go-to-line, `@` outline, mobile numeric go-to-line and a disabled-with-reason command list (U-PAL-2); plan Task 1.3.5b implements `>` command mode and ARIA audit only. Resolution: extend 1.3.5b/AC or trim ux S12.
**Status: ACCEPTED follow-up. Story 1.3.5 ships `>` command mode; ux S12 `:`/`@` modes and mobile numeric go-to-line are post-release polish**

**C11. Search UI behaviours present only in ux.** Cancelled state ("Search cancelled - 312 results so far", U-SEARCH-7), slow-search elapsed text, RPC-failure Retry, "Add filters" link from the truncated message (U-SEARCH-5), selection prefill, Ctrl+Enter pin. Story 1.4.1/1.4.2 ACs cover queued, invalid regex, zero results, truncated, beyond-portion, but not these. Resolution: fold into 1.4.2 ACs (or mark ux items optional).
**Status: RESOLVED for cancelled / RPC-failure Retry / Add-filters (one new Story 1.4.2 AC); ACCEPTED follow-up for selection prefill and Ctrl+Enter pin**

## NITPICK

- **N1 (RESOLVED: now "Story 0.1.1").** Plan Unresolved Questions and ADR-030 cite "Task 0.1.1"; only Story 0.1.1 and Tasks 0.1.1a/b exist.
- **N2.** Glossary `ContentSearcher` shows `Search(ctx, root, SearchQuery, emit)`; Task 1.1.1b and everything else use `SearchOptions` (the glossary note explains it; rename to avoid the alias).
- **N3.** `SearchOptions.Limits.ContextLines` and `word_mode` appear in the proto/glossary, but no AC or ux surface uses context lines; requirements ask only for "regex/case/glob options". Remove `ContextLines` or add a surface.
- **N4.** ux "Rejected (> 10 MB)" banner shares the name `Rejected` with the `SaveOutcome.Rejected(reason)` arm; rename the banner (e.g. "Too large to display").
- **N5.** ux 9.3 "Diff too large (> 5,000 changed lines)" row contains a garbled clause ("Download both versions is not available") and has no plan AC; Story 2.3.1 refers to "the existing deleted-file variant" that is defined only in ux 9.3.
- **N6.** ADR-034 describes `@codemirror/merge` showing result-vs-theirs; ux S8 desktop wireframe shows Base | Mine | Theirs | Result panes. Reconcile the layout description.
- **N7.** Several ux items have no task: tab reorder via Alt+Shift+Arrow (U-TAB-4), "not found" persisted tab and "Close all missing" (U-TAB-6), font-size persistence per device (U-MOB-6), long-press back showing jumplist, session-paused banner (7.2), shortcut overlay rows for F4, Ctrl+W, Ctrl+B, Esc-then-Tab (Story 1.3.5 AC lists only five shortcuts).
- **N8.** Line-budget check is split: Task 1.2.2d (jest assertion for `FilesTab.tsx`) vs. plan text "ACs of 1.2.1 and 2.5.1" (both use `wc -l`); the plan prose says "asserted by a jest check" in several places. Pick one mechanism.
- **N9.** Requirements success metric "No regression in existing Files-tab e2e/unit tests" is covered only indirectly (Story 1.2.1 AC names `FileContentViewer.test.tsx` and `local-file-browser.spec.ts`; Story 1.2.2 "Back-compat"). An explicit AC naming the existing FilesTab specs would close it.

## Coverage and drift summary

- **Requirements -> plan coverage**: every In Scope bullet has stories (search 1.1/1.4; multi-file 1.2/1.3 with split view explicitly deferred in both documents; editing/conflict/3-way merge 2.1-2.3; vim and touch 2.4; mobile 1.5; tests 1.4.3/1.5.3/2.4.3). NFRs: performance (1.1.3, 1.2.1d), security (0.2.1, 2.1.x), observability (1.1.2b, 2.1.2d), risk control (0.2.2, Risk Control). No requirement without a story.
- **Scope drift** (plan items with no requirement): `files:viewer-v2` kill-switch retirement backlog item, `StatFile`/staleness banner (1.3.4), `SearchContent` walk semaphore/`Queued` state, outline languages beyond need. All are justified by research and review; none contradicts requirements. `word_mode`/`ContextLines` are the only unused additions (N3).
- **Requirements text drift**: requirements.md In Scope marks split view as DEFERRED (consistent with the plan); its flag names still use underscores (C1).

N2-N9 are ACCEPTED as non-blocking nitpicks (tracked here; fix opportunistically during implementation).
