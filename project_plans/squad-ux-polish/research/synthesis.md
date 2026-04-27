# Research Synthesis: Squad UX Polish

**Date**: 2026-04-17
**Research files**: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

---

## Decision Required

Choose the design for three mutually-reinforcing UX improvements to stapler-squad: (1) batch/multi-session creation, (2) prompt injection at session creation, and (3) a review queue with PR creation — and decide whether to introduce a first-class "Project" grouping concept.

---

## Context

Firsthand user testing (Rich's MDD tutorial) confirmed that creating N parallel sessions for the same repo requires N separate form fills. Prompts cannot be loaded at creation time. When sessions complete, there is no integrated path to create a PR or review the work. The "project" concept (named group of sessions) emerged naturally during that test as something users mentally model but the UI doesn't surface.

The codebase already has: ent/SQLite for structured data, a review queue UI (`ReviewQueuePanel`), `ReactiveQueueManager` with `ReasonTaskComplete`, PR status polling, a `gh` CLI integration pattern in `github/client.go`, and `AutocompleteInput.tsx` for dropdowns. No new dependencies are needed for any of the three features.

---

## Options Considered

| Option | Summary | Key Trade-off |
|--------|---------|---------------|
| **Batch textarea** | Multi-line textarea, one task per line → N sessions | No complex UI; requires server-side concurrency control |
| **Client-side N calls** | Frontend calls `CreateSession` N times | Simpler server; no partial-failure visibility; no throttle point |
| **New `BatchCreateSessions` RPC** | Server manages concurrency + partial results | More work but correct semantics |
| **Prompt via CLI flag** | Use `claude --system-prompt "..."` at spawn time | Works but exposes prompt in process list |
| **Prompt via temp file** | Write prompt to temp file, pass path as CLI arg | Avoids process-list exposure; already the pattern for CLAUDE.md |
| **Prompt via tmux send-keys** | Send prompt after session starts | Race condition confirmed; 255-byte limit for PTY path |
| **Project as first-class ent entity** | `Project` table, nullable FK on `Session` | Most flexible; clean migration (null FK = no project) |
| **Project as tag convention** | Tag sessions `project:my-name` | Zero schema change; breaks down for project-level actions |
| **Review queue as new view** | "Review" nav item, filtered `ReasonTaskComplete` sessions | No new state machine changes needed |
| **Review queue as separate entity** | New ReviewItem entity with its own lifecycle | Overengineered for v1 |

---

## Dominant Trade-off

**Correctness vs. simplicity for batch creation.** Client-side N calls are simpler but give no partial-failure visibility and no server-side concurrency control. The pitfalls research confirmed that concurrent `git worktree add` calls share the repo's object store and `.git/index.lock` — stale locks from crashes block all future operations. The server must serialize or bound-concurrency the worktree creation step, which means the server needs to own batch orchestration.

**For the other two features**, the trade-off is simpler: the infrastructure already exists. Prompt injection and review queue are integration work, not design choices.

---

## Recommendation

### Batch session creation: `BatchCreateSessions` RPC with bounded sequential worktree creation

**Choose**: New `BatchCreateSessions(repeated BatchSessionRequest) → BatchCreateResponse(repeated BatchCreateResult)` ConnectRPC endpoint.

**Because**: git `worktree add` is not safe for concurrent calls on the same repo (confirmed: stale `.git/index.lock` blocks all future git ops). The server must serialize or bound-concurrency worktree creation. Client-side N calls cannot provide this guarantee. A `BatchCreateResult` per item gives callers structured partial-failure information instead of a single opaque error.

**Implementation**: Reuse existing `CreateSession` handler logic per item. Add a server-side worker pool (max 3 concurrent worktree additions). Append `-N` or a 6-char hex suffix to session title before sanitization to prevent tmux naming collisions. Return `[]BatchCreateResult{ID, Title, Error}` — callers show a per-session status list.

**UI**: "Batch" tab on the new-session form. Textarea (one task per line). Preview shows N session titles. Concurrency limit label ("max 20 per batch"). Disable submit if >20 lines.

**Accept these costs**: New RPC adds protobuf schema + codegen overhead. Sequential worktree creation means batch of 10 takes ~10× single session time.

**Reject**:
- Client-side N calls: rejected because no server throttle, no structured partial failure, no lock protection
- Parallel worktree creation: rejected because confirmed `.git/index.lock` race condition risk

---

### Prompt at creation time: temp-file delivery + prompt history JSON

**Choose**: Add `InitialPrompt string` to the session creation form and the `CreateSession` RPC. Write the prompt to a temp file in the worktree before starting the Claude process. Pass it via a mechanism that doesn't use `tmux send-keys` (confirmed race condition + 255-byte PTY limit for that path). Persist recently-used prompts in `~/.stapler-squad/workspaces/{hash}/prompts.json` (cap 500 entries, ring-buffer, `LastUsed` + `UsedCount` fields).

**Note from web search**: Claude Code CLI does not have an `--init-prompt` flag. The correct delivery mechanism is either `--system-prompt` (exposes prompt in process list) or a CLAUDE.md injection (write to worktree's CLAUDE.md before start). The CLAUDE.md injection approach is cleanest — it uses Claude Code's own context loading, leaves no process-list exposure, and is already how `session/instance.go` handles other per-session context.

**Implementation**: Before starting the tmux session, if `InitialPrompt` is set, append it to (or create) `<worktree>/.claude/session-prompt.md` and add an `@.claude/session-prompt.md` import line in the worktree's CLAUDE.md. The file is ignored by git (add to `.gitignore` on worktree creation).

**UI**: `InitialPrompt` textarea on the existing creation form (not a new modal). "Recent prompts" dropdown above it using `AutocompleteInput.tsx`. File upload button (read file contents into textarea).

**Accept these costs**: Worktree CLAUDE.md modification means prompt content ends up in the CLAUDE.md — visible to the user and to any AI linting that reads CLAUDE.md.

**Reject**:
- `tmux send-keys` delivery: rejected because confirmed race condition (session must be ready) and 255-byte PTY path limit
- `--system-prompt` flag: rejected because exposes prompt content in process list (`ps aux`)

---

### Review queue: one-shot agent-driven PR creation

**Choose**: "Create PR" button in the review queue sends a one-shot prompt to the session's AI agent instead of shelling out `gh` from the server. The agent runs non-interactively in its worktree, creates the PR with its full session context, and terminates.

**Why**: The agent has context about what it changed and can write a meaningful PR description. It handles auth failures, branch protection, and conflicts autonomously. It works across programs (Claude, Aider, etc.). Server-side `gh` shell-out requires the server to manage push, auth, and error parsing — the agent already does all of this.

**Mechanism — `claude -p "prompt"` (confirmed)**: Claude Code supports a non-interactive one-shot flag: `claude -p "Create a pull request..."`. This runs in the session's worktree directory, executes the task, and exits. The server spawns this as a subprocess, captures stdout for the PR URL, and updates the session record.

**Default one-shot PR prompt** (user-editable):
```
Create a pull request for your current branch. Push the branch if it isn't already pushed.
Write a clear title and description summarizing your changes based on your session context.
Use 'gh pr create' or your preferred method. Output the PR URL on the last line.
```

**Implementation**:
1. Add `RunOneShot(session_id, prompt) → RunOneShotResponse{output, error}` ConnectRPC endpoint — general-purpose, not PR-specific
2. "Create PR" button fills the default PR prompt in a confirmation modal (user can edit before running)
3. Server runs `claude -p "<prompt>"` in the session's worktree directory (or uses the session's configured program if not Claude)
4. Parse last line of output for PR URL; on success update session's `GitHubPRURL` field
5. PR status poller picks up the URL automatically (already wired)

**One-shot generalizes to the full "one-shot workflow" requirement**: The same `RunOneShot` endpoint powers future pre-defined actions (run tests, deploy, update docs, etc.) — any action the agent can do in one pass.

**Prompt delivery for interactive sessions with initial prompt (separate feature)**:
- `send-keys` is safe for sessions **already running** (Claude at its interactive prompt, waiting for input) — the race condition only applies to startup
- For startup injection: use `claude -p "prompt"` for one-shot sessions; for interactive sessions, append to worktree CLAUDE.md before launch (Claude loads it at startup) OR use `send-keys` with a readiness poll (wait until tmux pane output contains Claude's prompt indicator before sending)

**Accept these costs**: One-shot PR creation takes 10–30s (agent running) vs <1s for a direct `gh` shell-out. User sees a spinner. Output is less predictable than a structured API response.

**Reject**:
- Server-side `gh pr create` shell-out: rejected because agent already has shell access, context, and handles errors better; server-side approach requires managing push + auth + error parsing separately
- AI-assisted code review before merge: deferred to v2

---

### Project concept: first-class ent entity with nullable FK

**Choose**: Add `Project` as a new ent schema entity. Add nullable `project_id` FK on `Session`. Existing sessions keep null FK (no project = not breaking). Projects have: ID, Name, Description, WorkspacePath, CreatedAt.

**Because**: Tag convention breaks down for project-level actions (e.g., "pause all sessions in this project", "create PR for all complete sessions in this project"). A first-class entity is queryable and enables aggregate operations. The ent schema already uses SQLite; adding a table is schema + codegen, not a dependency change.

**Migration**: `ent.Schema.Create(ctx)` with SQLite will add the new table and FK column without wiping existing data (ent uses `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN` semantics for SQLite). No manual migration SQL required.

**UI**: "Project" picker on session creation form. Project grouping strategy in the existing `GroupBy` dropdown. Project header in the session list with aggregate stats (N running, N complete, N ready for review).

**Accept these costs**: Schema codegen required after adding entity. UI needs project CRUD (create/rename/delete).

**Reject**:
- Tag convention: rejected because it can't support aggregate queries or project-level actions without scanning all sessions
- Repo-path auto-grouping: rejected because users want named projects that may span multiple repos or subdirectories

---

## Open Questions Before Committing

- [ ] **CLAUDE.md injection for prompts**: Confirm that an `@path` import in a worktree's CLAUDE.md is resolved relative to the worktree root (not the host project root) — blocks prompt delivery design
- [ ] **PTY limit for batch**: Determine the actual macOS PTY limit for the current CI/dev machine (`sysctl kern.tty.ptmx_max`) — blocks max batch size decision
- [ ] **One-shot program selection**: For sessions running Aider or other programs, what is the one-shot invocation? Does Aider support a `-p` equivalent? Should `RunOneShot` always use `claude -p`, or use the session's configured program?
- [ ] **Claude -p in worktree**: Confirm `claude -p "..."` run in a worktree directory picks up the worktree's CLAUDE.md (not the host repo's) — affects context available to the PR creation prompt

---

## Sources

- [findings-stack.md](findings-stack.md) — stack options, ent/SQLite pattern, existing AutocompleteInput
- [findings-features.md](findings-features.md) — Gastown/Cursor/claude-flow feature survey
- [findings-architecture.md](findings-architecture.md) — data model options, state machine integration
- [findings-pitfalls.md](findings-pitfalls.md) — batch creation races, prompt timing, PR auth
- [gh pr create issue #6366](https://github.com/cli/cli/issues/6366) — confirmed: `gh pr create --json` not supported; use stdout URL + `gh pr list` fallback
- [git worktree concurrent safety](https://github.com/kaeawc/auto-worktree/issues/176) — confirmed: concurrent `worktree add` unsafe; stale `.git/index.lock` blocks all operations
- [Claude Code CLI reference](https://code.claude.com/docs/en/cli-reference) — confirmed: no `--init-prompt` flag; use CLAUDE.md or `--system-prompt`
