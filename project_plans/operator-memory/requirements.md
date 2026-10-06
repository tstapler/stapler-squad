# Requirements: Persistent Operator Memory

Source: backlog item `2da5cd02-5b45-4f75-a053-b33bbb3e3792` (migrated from
`TylerStaplerAtFanatics/stapler-squad#116`).

## Problem

Every headless triage/work/review call in Stapler Squad starts cold. There is
no mechanism to persist learned institutional knowledge — team conventions,
recurring failure patterns, codebase quirks, preferred approaches — across
backlog items or sessions. Each headless call sees only the stable system
prompt (`session/headless/features.go`) plus the per-call item/diff payload.

## Reference design (Hermes Agent `tools/memory_tool.py`)

- Two file-backed stores: `MEMORY.md` (learned facts about the environment)
  and `USER.md` (learned facts about the user).
- Both loaded as a **frozen snapshot at session/call start** and appended to
  the system prompt. Mid-session writes update the file on disk but do not
  change the running prompt — the next session picks up the update.
- Content is scanned for prompt injection before being written.
- The snapshot is the final, volatile tier of the system prompt (after the
  stable identity/rules tier), so a snapshot change doesn't invalidate the
  cacheable stable prefix — only the tail.

## Scope for this item

This issue creates the **storage + injection layer only** — read-side. It
does not build the writer that populates the stores; a companion backlog item
covers the background post-completion reviewer that writes to them
automatically. Until that lands, `OPERATOR.md`/`REPO.md` are operator-edited
by hand (`stapler-squad memory` CLI or direct file edit).

## Functional requirements

1. **`~/.stapler-squad/memory/OPERATOR.md`** — fleet-level knowledge (team
   conventions, cross-repo patterns), one file, not per-workspace.
2. **`<workspace-config-dir>/memory/REPO.md`** — repo/workspace-specific
   knowledge (build quirks, test patterns, known flaky areas). Lives per
   workspace, consistent with this repo's existing workspace-isolated config
   convention (`config.GetConfigDirForDir`, `STAPLER_SQUAD_TEST_DIR`).
3. Both files are loaded once per headless call as a frozen snapshot and
   appended as a new tail block to the existing stable system prompt used by
   `BuildHeadlessTriagePrompt`/`BuildHeadlessRetriagePrompt` (triage) and
   `BuildReviewPrompt`/`BuildHeadlessReviewPrompt` (review). The existing
   stable prompt constants in `session/headless/features.go` are unchanged;
   memory is a new suffix, not an edit to the existing text.
4. `stapler-squad memory show` CLI subcommand displays current contents of
   both stores (operator + repo-local, for the CWD's workspace).
5. Empty memory files (missing, zero-length, or whitespace-only) produce
   **no** system prompt block at all — no `## Operator Memory` heading with
   empty body, no noise.
6. Content is scanned for prompt injection **before any write** to either
   file. Scope this to whatever write path this issue actually introduces —
   if this issue only ships a manual-edit/CLI-view path with no CLI *write*
   command, state that explicitly rather than building an unused scanner (see
   open question in plan.md about `memory edit`/`memory add`).

## Acceptance criteria (from the original issue, carried forward)

- [ ] `OPERATOR.md` and `REPO.md` loaded at session start, injected into
      system prompt as a frozen snapshot
- [ ] Injection scan runs before any write to either file
- [ ] `stapler-squad memory show` displays current contents
- [ ] Empty memory files produce no system prompt noise (graceful
      empty-state)
- [ ] Unit tests: prompt assembly with populated memory, prompt assembly with
      empty memory, write-scan rejection

## Non-goals

- The automatic background-reviewer writer (separate backlog item).
- Memory injection into *interactive* (non-headless) Claude Code sessions
  spawned by Stapler Squad — CLAUDE.md already covers that path; this issue
  is scoped to the headless triage/review call path only, per the issue
  description's explicit references to `BuildTriagePrompt`/
  `BuildReviewPrompt`.
- Memory size management / summarization / eviction policy — out of scope
  until a real growth problem is observed (ponytail: start with a byte cap,
  not a summarization pipeline).
