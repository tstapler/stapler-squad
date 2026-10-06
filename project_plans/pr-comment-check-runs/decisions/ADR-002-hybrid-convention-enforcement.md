# ADR-002: Hybrid Enforcement — Doc Convention + Session-Scoped Go Primitive + Standalone Script (No Mandatory RPC Gate)

## Status
Accepted

## Context

Automation that touches GitHub PRs in this repo is not one code path — it's several independent ones, confirmed by `research/architecture.md` §1 and `research/features.md` §1:

- **Ad hoc `gh`-CLI-driven Claude Code skill sessions** — `github-pr`, `pr-ship`, `pr-refine`, `code:review`, `github-address-pr-comments`. These call `gh pr ...`/`gh api ...` directly from Bash. None of them resolve a stapler-squad session ID or call a ConnectRPC endpoint; `github-pr`'s own SKILL.md documents zero Go/RPC/MCP calls in its patterns.
- **Backlog-driven autonomous shepherding** — the one flow that *is* a long-lived `session.Instance`, with `GitHubOwner`/`GitHubRepo`/`GitHubPRNumber` already populated as struct fields, and which already calls Go RPCs (`report_progress`, `submit_review_verdict`, `report_pr_created` per the MCP tool surface).
- **The Go backend's own deliberate write** — `forwardSyncCloseComment`, explicitly out of scope for this feature.

Three shapes of "where does the comment-vs-status decision get enforced" were considered (recorded in `plan.md`'s Pattern Decisions Creative pass):

1. Route every GitHub write, from every caller, through one mandatory Go RPC that itself decides comment-vs-status.
2. Write the policy down in `github-pr/SKILL.md` and add no new Go capability at all.
3. A hybrid: the policy lives as documented convention (both caller types read it), plus each caller type gets a primitive shaped for how it already talks to GitHub — a session-scoped Go method (`Instance.SetCommitStatus`) for the one flow that's already Go-session-scoped, and a standalone script (`pr-status.py`) for everything else.

Option 1 was rejected because it would require every ad hoc skill invocation to first resolve a stapler-squad session ID just to post a status — a bigger architecture change than this ticket's scope, and the requirements doc explicitly rules out "redesigning backlog automation end-to-end."

Option 2 was rejected because it leaves the one flow that already runs as a typed, tested Go session (backlog shepherding) with no typed, tested way to write a status — it would have to hand-roll `gh api` calls inline, with the SHA-staleness handling (`pitfalls.md` §1d: a status written against a rebased-away SHA is a silent no-op) reimplemented ad hoc, with no test coverage, every time.

`research/pitfalls.md` §2–3 raises the deeper issue underlying this decision: **there is no code-level enforcement mechanism available for "should this session comment or set a status" at all.** Unlike this repo's `interface-pollution-checklist.md`/`primitive-obsession-checklist.md`, which work because the smell lives in Go source a linter or `go build` can inspect, "should I comment or should I emit a status here" is a runtime judgment call made by an LLM-driven skill session reading prose — no compiler, type system, or lint rule can check it. Whatever gets built, the enforcement is advisory by construction.

## Decision

**Adopt the hybrid (Option 3).** Concretely, three things ship together, and none of them is "the" enforcement mechanism on its own:

1. **Policy**: `~/.claude/skills/github-pr/references/comment-vs-status-convention.md`, linked from `github-pr/SKILL.md` and from one-line pointers added at the specific points in `pr-ship`/`code:review` where a future edit might add new GitHub-status-reporting behavior (`plan.md` Story 1.1.2). This is the only artifact that actually states the rule; it is read-and-followed by a fresh agent, not compiled or linted against, and is explicitly acknowledged as advisory.
2. **Go primitive**: `github.SetCommitStatus` → `Instance.SetCommitStatus` → `GitHubService.SetCommitStatus`, for the one flow (backlog shepherding) that already runs as a `session.Instance` and can reasonably be expected to call a typed Go method instead of shelling out.
3. **Script**: `pr-status.py`, shipped as a real file in `github-pr/scripts/`, for every ad hoc `gh`-CLI-driven skill session that has no session ID to resolve.

## Consequences

### Positive
- Matches how automation actually runs today rather than assuming a single code path that doesn't exist. No skill has to change *how* it talks to GitHub (still `gh` CLI for ad hoc sessions, still Go method calls for session-scoped code) — only *what* it calls for status-shaped output.
- The SHA-staleness fix (`pitfalls.md` §1d) is implemented once, correctly, in `Instance.SetCommitStatus` (always calls `RefreshPRInfo()` first), and `pr-status.py` re-derives the same guarantee independently for its callers, rather than being skipped entirely by the flows that would otherwise hand-roll `gh api`.
- Nothing forces an out-of-scope redesign of backlog automation's control flow or of how skills invoke `gh`.

### Negative
- **Two implementations of "set a commit status"** (a Go function, a Python script) that have to stay behaviorally consistent by hand — there is no shared code between them, only a shared GitHub REST contract. This is accepted because `research/build-vs-buy.md` §3 found no real algorithm or business logic in this capability at all: both are thin wrappers around one fixed, GitHub-documented POST payload (`state`, `context`, `description`, `target_url`), so the drift surface is small and mechanical (a new field added to one and forgotten in the other), not behavioral.
- **No compiler/lint guard against reversion.** A future edit to `pr-ship` or `code:review` could reintroduce comment-heavy status narration and nothing would catch it automatically — `pitfalls.md` §2's "silent reversion risk" finding applies in full. The one-line pointers added in Story 1.1.2 are a mitigation (redirect the editor to the convention doc at the exact point they'd add new behavior), not a guarantee. A periodic audit (in the shape of this repo's existing `backlog-feature-improvement` skill, per `pitfalls.md` §3) is the realistic follow-up mechanism if drift is later observed — not built as part of this feature, since the requirements doc doesn't ask for one and inventing a new audit skill here would be scope creep beyond "reduce noise, add a status-writing capability."
- Advisory enforcement means AC1 ("a clear, written convention exists ... and is followed") can be satisfied at ship time but can silently stop being true later with no automated signal. This is named explicitly rather than papered over — see `plan.md`'s Risk Control section.

### Neutral
- If a future ticket decides the drift risk above is unacceptable, the natural next step is *not* Option 1 (a mandatory RPC gate) but a periodic audit skill (Option (a) from `pitfalls.md` §3) that samples recent shepherded-PR comment activity and flags regressions — a detection mechanism layered on top of this ADR's hybrid, not a replacement for it.
