# Findings: Features

**Subtopic**: Comparable tool survey — batch session creation, prompt-at-start, and review/merge queue UX patterns
**Date**: 2026-04-17
**Researcher**: subagent (Features)
**Target features**: (1) Batch/multi-session creation, (2) Prompt at session creation, (3) Review queue + one-click PR

---

## Summary

Every mature multi-agent tool has converged on three interrelated patterns for the problems stapler-squad is trying to solve:

1. **Batch creation via task decomposition, not form multiplication** — you describe the work (text list, YAML, structured task spec) and the tool creates N sessions in one shot. No one makes users fill a form N times.
2. **Prompt injection at spawn time** — work is attached to the session before it starts, either via a startup command, a context file baked into the session, or a "prime" step that the agent reads before beginning. The UI separation between "create session" and "send first message" is not user-friendly and has already been solved elsewhere.
3. **Review queue as a first-class view** — completed/awaiting-review work is surfaced in a dedicated queue, not buried in the main session list. PR creation is initiated from this view. The most advanced tools (Gastown Refinery) fully automate merge; lighter tools (Cursor background agents) produce a PR and surface it for human review.

The dominant open questions for stapler-squad are: (a) how much merge automation is desirable vs. a human-review-first approach, and (b) whether to model the review queue as a session state filter or as a separate entity.

---

## Options Surveyed

### 1. Gastown / Refinery (Steve Yegge, 2025–2026)

Gastown is an open-source multi-agent orchestration system managing 20–30 parallel Claude Code instances. The Refinery is its automated merge queue component.

**Batch creation**: Work is dispatched via `gt sling`, which targets a "rig" (a configured agent slot) and spawns a fresh "polecat" (worker agent). Multiple `gt sling` calls in sequence = batch creation. A configurable concurrency limit controls how many sessions are spawned simultaneously to avoid API rate exhaustion. The scheduler dispatches incrementally when a limit is set.

**Prompt at start**: The `gt prime` command is the core mechanism. When a polecat session starts, `gt prime` outputs the agent's role documentation, any "mail" (queued work), and system instructions as markdown to stdout. The agent reads this and begins immediately without waiting for a human to type anything. Work is stored in a "bead" (durable state) attached to a "hook" before the session starts — i.e., the task is fully specified before the agent is spawned.

**Review/merge queue (Refinery)**: When a polecat finishes, it runs `gt done`, pushing its branch and sending a merge request to the Refinery's inbox. The Refinery:
- Pulls the branch and runs the test suite
- If tests fail: rejects, returns issue to backlog for re-dispatch
- If tests pass but merge conflicts exist: attempts conflict resolution, re-runs tests
- If clean: merges to main automatically

This is fully automated with safeguards (no `git push --force`, no direct commits to main). Failed MRs are never silently dropped — they return to the queue.

**Key lesson for stapler-squad**: Gastown separates "task specification" (a durable record with title, description, context) from "session lifecycle" (the tmux process). The task exists before the session; the session reads the task at start. This decoupling is the right model. The Refinery's bisecting queue is overkill for stapler-squad's scale, but the "test gate before merge" concept is worth preserving as a one-click option.

---

### 2. Cursor Background Agents (2026)

Cursor ships background agents in February 2026 as part of a broad industry convergence (Windsurf, Claude Code Agent Teams, Devin all shipped similar features in the same two-week window).

**Batch creation**: In Cursor 3, Agent Tabs allow side-by-side or grid view of multiple agents running in parallel across local, worktree, cloud, and remote SSH environments. Up to 8 background agents can run simultaneously. There is no "create N agents from a task list" flow — each agent is started individually via `Ctrl+E`, but switching between them is a first-class UI concern.

**Prompt at start**: Background agents are triggered by a prompt submitted at creation time (the `Ctrl+E` → prompt input flow). There is no separation between "create agent" and "give it a task." This is the natural model. The agent runs autonomously on the cloud-cloned repo.

**Review/merge queue**: Upon completion, each background agent generates a PR with a summary of its changes. Cursor provides a built-in diff viewer. The "review queue" in Cursor is effectively GitHub's own PR list — Cursor doesn't maintain a first-party queue UI, it delegates to GitHub. Users have explicitly requested a queue feature in Cursor's forums (thread: "Cloud Agent queue") because the current model doesn't show which agents finished and need attention.

**Key lesson for stapler-squad**: The gap Cursor users are reporting — wanting a queue of completed agents to review — is exactly the review queue feature in scope. Cursor outsources this to GitHub; stapler-squad can own it natively by tracking session state machine transitions to "completed" and surfacing them in a dedicated filtered view. The prompt-at-start UX (one combined prompt+create flow) is the right model.

---

### 3. Claude Flow / Ruflo (ruvnet, 2025–2026)

Originally claude-flow, renamed Ruflo in January 2026. The leading open-source multi-agent orchestration platform for Claude Code, with 31,100+ GitHub stars and 6,000+ commits.

**Batch creation**: Ruflo uses `claude-flow agent spawn --type <type> --name "<name>"` to create individual agents. It supports hierarchical (queen/workers) and mesh (peer-to-peer) topologies. The orchestrator decomposes a top-level task into subtasks and spawns specialist agents for each. This is task-decomposition-driven batch creation, not form-driven.

**Prompt at start**: The orchestrator passes the initial prompt to each spawned agent. However, a known limitation (per GitHub discussion #692): `agent_spawn` via the MCP tool creates a Map entry but the prompt is stored and not immediately processed. For complex multi-paragraph prompts, users are advised to use Claude Code's native Agent tool instead. This is a signal that prompt delivery timing is a real problem even in mature systems.

**Review/merge queue**: Ruflo does not have a native review queue or merge flow. It is focused on task execution, not on the landing/review step. Users must handle PR creation and review externally.

**Key lesson for stapler-squad**: Even in the most-starred Claude orchestration tool, prompt delivery at spawn time is partially broken and acknowledged as a limitation. The timing problem (session must be ready before the prompt arrives) is real and requires explicit handling. The absence of a native review queue in Ruflo is a gap that stapler-squad can fill advantageously.

---

### 4. tmuxinator / tmuxp

tmuxinator (Ruby) and tmuxp (Python) are the canonical tmux session template tools.

**Batch creation**: Both work by defining a session layout in YAML, then executing it with one command (`tmuxinator start <project>` or `tmuxp load <config>`). A single YAML config can define multiple windows, each running a different command. To create "N sessions from the same template," you can define one config as a template and run it with different project-name arguments. tmuxp can also "freeze" a running session into a config (capturing the inverse direction).

**Prompt at start**: In tmuxinator/tmuxp terms, "prompt at start" maps to the `command` key on a window or pane — this is the shell command that runs immediately when the pane is created. This is structurally equivalent to the `gt prime` mechanism: the session starts with a defined command, not an empty prompt.

**Review/merge queue**: N/A — tmuxinator/tmuxp are session lifecycle tools with no concept of session completion, review, or merge.

**Key lesson for stapler-squad**: The YAML template model (one config → N sessions, parameterized) is validated and widely adopted. For stapler-squad's template/batch feature, storing session templates as structured data (JSON in the existing config, or a `templates.json` next to `sessions.json`) follows this proven pattern. The `command` → `initial_prompt` analogy maps cleanly to stapler-squad's model.

---

### 5. GitHub CLI (`gh pr create`)

`gh pr create` is the canonical terminal-native PR creation workflow.

**Batch creation**: N/A — `gh` operates on one PR at a time per invocation.

**Prompt at start**: N/A — not applicable to PR creation tools.

**Review/merge flow**: `gh pr create` is ergonomic by design:
- Without flags: interactive prompts ask for title, body, assignees, labels, reviewers
- With `--fill`: autofills title and body from commit history (zero manual input for well-crafted commits)
- With `--title` and `--body`: fully non-interactive
- If the branch isn't pushed, `gh` prompts to push it first and then continues — one combined flow

The key ergonomic win of `gh` is that it collapses the "push branch + open GitHub + fill form + submit" workflow into a single terminal invocation. For stapler-squad's "one-click PR creation" feature, the backend should call `gh pr create --fill` (or the GitHub API equivalent) pre-populated from the session's branch, title, and commit history. The user sees a confirmation dialog, not a form.

**Key lesson for stapler-squad**: The `--fill` flag is the model for low-friction PR creation. Pre-populate everything from available context (branch name, last commit message, session title); ask the user only to confirm. The review queue UI should show the pre-filled PR draft and let the user submit with one click.

---

### 6. Linear — Batch Issue Creation and Keyboard-First UX

Linear is the fastest-keyboard issue tracker; its UX patterns are worth studying for any batch-creation flow.

**Batch creation**: Linear's approach:
- `C` to create a single issue from anywhere
- `Option/Alt + C` to create from a template
- Multi-select with `X` (hover) or `Shift + click`, then bulk-action from the action bar
- `Cmd/Ctrl + A` selects all issues in the current filter view for bulk operations

There is no native "paste a list of tasks → create N issues" flow in Linear — bulk creation is done by creating issues one at a time (fast with keyboard shortcuts) and then multi-selecting for bulk property assignment (status, assignee, label).

**Prompt at start / template**: `Option/Alt + C` launches a "create from template" dialog that pre-fills all fields from a named template. This is the UI pattern for stapler-squad's template-based session creation.

**Review/merge queue**: N/A — Linear tracks issues, not code; it has no merge flow.

**Key lesson for stapler-squad**: The "paste a list → create N" use case is not well-served by any existing tool's UI. Stapler-squad has an opportunity to be genuinely better here. The right interaction model is: a multi-line text area in the new-session form, one task per line, with a "create N sessions" preview showing what will be created. This is a novel interaction pattern validated by absence — nobody else has done it well.

---

## Trade-off Matrix

| Tool | Batch Creation UX | Prompt-at-Start | Review/Merge Workflow | Discoverability | Adoption Friction |
|------|-------------------|-----------------|----------------------|-----------------|-------------------|
| **Gastown** | Excellent — `gt sling` dispatches tasks with durable task specs pre-baked | Excellent — `gt prime` injects role + task at session start automatically | Excellent — Refinery auto-merges with test gates; bisecting queue for failures | Low — CLI-only, steep learning curve | High — full orchestration system, significant setup |
| **Cursor BG Agents** | Good — 8 parallel agents, but each started individually via UI | Excellent — prompt is the trigger; no separation between create and task | Fair — delegates to GitHub PR; no native review queue (users want one) | High — integrated into IDE, `Ctrl+E` is discoverable | Low — IDE-native, zero extra tooling |
| **Claude Flow / Ruflo** | Good — orchestrator decomposes and spawns automatically | Fair — prompt stored at spawn but not reliably delivered in MCP path | Poor — no native review/merge flow at all | Medium — popular but requires CLI knowledge | Medium — npm install, but API complexity |
| **tmuxinator/tmuxp** | Good — YAML templates, one-command session restore | Fair — `command` key fires a shell command at pane start | None | Medium — well-documented, Ruby/Python-familiar | Low — single install, YAML config |
| **GitHub CLI** | N/A | N/A | Excellent — `--fill` autofills from commits; interactive with good defaults | High — `gh` is ubiquitous | Very low — already installed for most devs |
| **Linear** | Fair — fast single creation (C shortcut), no paste-list-to-bulk | Good — template creates pre-filled forms | N/A | High — keyboard shortcuts discoverable via help menu | Very low — SaaS, no setup |
| **stapler-squad today** | Poor — one form per session | Poor — must navigate to session post-creation to type | None — manual, outside the tool | Medium — web UI exists | Low — already installed |
| **stapler-squad target** | Excellent — paste task list → N sessions preview → confirm | Excellent — text area + clipboard + recents in creation form | Good — completed sessions queue + one-click `gh pr create --fill` | High — review badge on nav, "Add Tasks" button prominent | None — in-product, existing users |

---

## Risk and Failure Modes

### Batch Creation Risks

**Race conditions on simultaneous session creation**: If N sessions are created in rapid succession, tmux session naming can collide (if names are auto-generated) and git worktree creation can fail on filesystem races. Gastown mitigates this with a concurrency limiter and durable task queue — tasks are registered first, sessions are spawned incrementally. Stapler-squad should do the same: register all N sessions as pending records atomically, then spawn them sequentially or with a controlled concurrency limit (e.g., 3 at a time).

**"Ghost sessions"**: If the batch creation UI is too easy (paste → click), users may create 20 sessions they forget about. This is a real UX problem in multi-agent systems; Gastown's backlog model (tasks wait for capacity) partially addresses it. Stapler-squad should show a preview of "N sessions will be created" and require explicit confirmation.

**Template staleness**: Session templates that encode directory paths, branch names, or prompts become stale as projects evolve. No tool has a good solution for this. Mitigate with "last used" timestamps and validation before creation.

### Prompt Delivery Risks

**Timing failure**: The session must be attached (tmux session running, Claude Code started) before a prompt can be delivered. Ruflo's known bug — prompt stored but not delivered — stems from exactly this timing issue. Stapler-squad currently uses a similar approach and must handle the race explicitly.

**Prompt truncation**: Multi-paragraph prompts may get truncated or garbled if delivered via terminal input simulation rather than a proper API. Use the `tmux send-keys` approach with appropriate delays, or (better) use a CLAUDE.md injection mechanism or `--init-prompt` flag if Claude Code supports it.

**Clipboard contamination**: If "paste from clipboard" is supported, the clipboard content at creation time may not be what the user intended at session-start time. Capture and store the clipboard value immediately on paste.

### Review Queue Risks

**Stale/diverged branches**: A session that was "completed" weeks ago may have a branch that has diverged significantly from main. The review queue should show branch staleness (commits behind main) and warn before PR creation.

**Auth token scope**: `gh pr create` requires a GitHub token with `repo` scope. If the user's `gh` auth is not set up, the feature silently fails. Validate `gh auth status` at startup or before PR creation, and surface a clear error with remediation steps.

**False "completed" status**: A session whose Claude process exited (crash, timeout, rate-limit) may appear "completed" but did not finish its task. Distinguish between "agent exited normally" and "task was completed with a reviewable result." Use a sentinel (e.g., last output line contains a success marker, or a PR was actually created) rather than relying on process exit status alone.

---

## Migration and Adoption Cost

**Batch creation**: Zero migration cost — the existing single-session creation path is unchanged. Batch creation is an additive "Add Multiple" path in the creation flow. Users who don't use it are unaffected.

**Prompt at start**: Low migration cost — adding a text area to the existing session creation form. The text area can be empty (current behavior preserved). Existing sessions without initial prompts continue to work as-is. The prompt library (recents) is additive.

**Review queue**: Low migration cost — the review queue is a new view, not a replacement. Sessions currently in "completed" or "stopped" state can be backfilled into the queue on first load. Existing users will see their old completed sessions appear in the queue on upgrade; this is net-positive discoverability, not a breaking change.

**Session data model**: Adding `InitialPrompt string`, `Template string`, and `ReviewStatus string` (or using an enum) to the session struct requires a migration of the JSON sessions file, but the existing migration pattern (nil/zero values for new fields = backward compat) is already established in the codebase.

---

## Operational Concerns

**`gh` CLI dependency**: One-click PR creation depends on `gh` being installed and authenticated. This is a hard runtime dependency for that feature. Mitigate by: (a) checking at feature invocation, (b) showing a clear setup guide if missing, (c) not blocking other features on `gh` availability.

**Prompt storage size**: A prompt library storing recent prompts is trivially small (100 prompts × 1KB each = 100KB). Store in the existing config/state JSON under a `prompt_library` key. No new storage dependency needed.

**Concurrency for batch creation**: Creating 20 sessions at once will spawn 20 git worktree creation processes and 20 tmux sessions. This is O(N) disk I/O and O(N) processes. Cap batch creation at a configurable limit (default: 5 concurrent) and show a progress indicator. Gastown's concurrency limiter is the right model.

**Review queue polling**: If the review queue shows PR status (CI passing/failing), it needs to poll GitHub. GitHub's REST API rate limit is 5,000 requests/hour for authenticated users. For a reasonable session count, polling every 30 seconds per active PR is well within limits. Use `gh api` or the GitHub REST API directly; do not use GraphQL unless complexity demands it.

---

## Prior Art and Lessons Learned

**Gastown's "task-first, session-second" model is architecturally correct.** Every other tool that tried to conflate "create session" with "deliver task" hit timing and reliability problems (Ruflo's prompt delivery bug being the clearest example). The right model: create a durable task record, then attach a session to it. The session reads the task at start via a reliable mechanism (file, environment variable, or startup command), not via terminal input simulation.

**Cursor's users want a review queue and don't have one.** The explicit community request for a "Cloud Agent queue" in Cursor's forums validates the review queue as a real user need that is not yet met by any mainstream tool. This is a genuine competitive advantage for stapler-squad.

**`gh pr create --fill` is the right ergonomics model.** The UX of pre-populating everything from context and asking only for confirmation — rather than making users re-enter data they already expressed (branch name, commit message, session title) — is the correct design. The "fill from context" model is proven at scale.

**tmuxinator's YAML template model validates the template approach.** Named templates with stored default field values (command, directory, layout) that can be parameterized at launch time is a 10-year-old proven pattern. Stapler-squad's template feature should be simpler (JSON, not YAML), not more complex.

**Linear's `C` shortcut and template modal are the right keyboard patterns.** Fast keyboard-first creation with optional template expansion is a well-validated UX. For stapler-squad's web UI, the equivalent is: `N` to create a new session (already exists via omnibar), `T` to create from template (new), and the creation modal having a "Batch" tab alongside the "Single" tab.

**Nobody has solved "paste a task list → N sessions" well.** This is a genuine gap. The closest analogy is Linear's bulk issue creation by CSV import, but that's file-based and not suitable for the "quick task list" use case. The interaction pattern to build is: multi-line textarea → split on newlines → preview N session cards → confirm. This is novel and would be a meaningful UX differentiator.

---

## Open Questions

1. **Should the review queue integrate with GitHub PR status?** Showing CI pass/fail, review comments, and merge readiness in the queue view adds value but requires polling GitHub. Is this in scope for the first iteration, or should v1 be "local state only" (completed session + branch present) and v2 add GitHub integration?

2. **How should the review queue handle sessions that complete but never created a PR?** Some sessions may complete their task without needing a PR (e.g., research tasks, local-only changes). Should the review queue show all completed sessions, or only those with a git branch ready for PR creation?

3. **Prompt library: per-project or global?** Gastown stores task specs globally (the bead/hook model is not project-scoped). Linear stores templates per-team. For stapler-squad, a global prompt library (shared across all repos) is simpler to implement and avoids the "which project does this belong to?" question for Phase 1.

4. **What is the right batch creation limit?** Gastown uses a configurable concurrency limit (default unclear from docs). Cursor caps at 8 background agents. What limit is right for stapler-squad given local tmux + git resource usage? Suggested default: 5 concurrent, max 20 per batch operation.

5. **Should batch creation support heterogeneous prompts (each task different) or only homogeneous (same prompt, N times)?** The "paste task list → N sessions" model implies heterogeneous. The "fork/clone session" use case implies homogeneous. Both are valid; the UI should support both — a task list textarea for heterogeneous, and a "repeat N times" option for homogeneous.

6. **For `gt prime`-equivalent in stapler-squad: file injection or terminal send-keys?** Injecting the prompt via a startup file (a temporary CLAUDE.md or `--init-prompt` flag) is more reliable than `tmux send-keys` which has timing issues. Does Claude Code support any startup prompt mechanism via CLI flag or env var that stapler-squad can leverage?

---

## Recommendation

### Feature 1: Batch / Multi-Session Creation

**Adopt the Gastown task-first model with a Linear-style creation modal.**

- Add a "Batch" tab to the session creation form alongside the existing "Single" tab
- The Batch tab contains a multi-line textarea, one task title per line (with optional delimiter for title::prompt)
- On submit: create N session records atomically (persisted to disk), then spawn them with a concurrency limiter (default: 5 concurrent)
- Show a progress view during batch creation with per-session status (pending → starting → running)
- Cap single-batch operations at 20 sessions (configurable)
- Session fork/clone: add a "Fork Session" action on running session cards; copies branch base, prompt, template to a new session creation pre-filled form

### Feature 2: Prompt at Session Creation

**Add a prompt field to the single-session creation form immediately; no new modal needed.**

- Add a `InitialPrompt` text area below the existing title/directory/branch fields in the creation form
- Support three input modes with tab-switching: (a) Type/paste text, (b) Upload file, (c) Select from recent-prompts dropdown
- Persist recent prompts (last 20) in config state; display as a searchable dropdown
- Deliver the prompt via the most reliable mechanism available: prefer a temp file injected via `tmux send-keys` with `$(cat /tmp/session-prompt-<id>.txt)` or a CLAUDE.md startup hook, not raw character-by-character send-keys
- Empty prompt field = current behavior (no change for existing users)

### Feature 3: Review Queue

**Build the review queue as a filtered session view with a one-click PR creation action.**

- Add a "Review" section to the navigation (badge with count of sessions awaiting review)
- The review queue shows all sessions in "completed" or "stopped" states, sorted by completion time (newest first)
- Each queue card shows: session title, branch name, last commit message, how many commits ahead of main, time since completion
- "Create PR" button pre-populates a PR draft using `gh pr create --fill` logic: title from session title, body from last commit message + session prompt (if present), base branch from main
- Show a confirmation modal with the pre-filled PR details; user can edit before submitting
- "Dismiss from queue" to remove a session from the queue without creating a PR (for no-PR-needed completions)
- v1: local state only (no GitHub CI polling); v2 can add GitHub status integration
- One-click "Create All PRs" bulk action for multi-select in the queue view (Linear-style bulk action)

---

## Pending Web Searches

The following claims should be verified by the parent agent if time permits:

1. **Cursor `--fill` equivalent**: Does Cursor's background agent PR creation use commit-message autofill similar to `gh pr create --fill`, or does it ask the AI to write the PR description? URL to check: [https://docs.cursor.com/en/background-agent](https://docs.cursor.com/en/background-agent)

2. **Gastown concurrency limits**: What is the default concurrency limit for `gt sling` batch dispatch? Check the Gastown CHANGELOG or quick-start docs for the specific default value. URL: [https://github.com/gastownhall/gastown/blob/main/CHANGELOG.md](https://github.com/gastownhall/gastown/blob/main/CHANGELOG.md)

3. **Ruflo/claude-flow prompt delivery fix**: Has the MCP `agent_spawn` prompt delivery bug (discussion #692) been fixed in v3.5? Check: [https://github.com/ruvnet/ruflo/discussions/692](https://github.com/ruvnet/ruflo/discussions/692)

4. **Claude Code `--init-prompt` or startup file support**: Does Claude Code CLI support any mechanism to inject an initial prompt at startup (env var, flag, stdin, or CLAUDE.md startup section) that would let stapler-squad reliably deliver prompts without timing races? Search: `"claude code" "--init-prompt" OR "initial prompt" CLI flag startup`

5. **Gastown Refinery test gate details**: What test commands does the Refinery run before merging? Is it configurable per-project or hardcoded? URL: [https://deepwiki.com/steveyegge/gastown/1.2-quick-start-guide](https://deepwiki.com/steveyegge/gastown/1.2-quick-start-guide)

---

## Sources

- [Gas Town — Welcome post (Steve Yegge, Medium)](https://steve-yegge.medium.com/welcome-to-gas-town-4f25ee16dd04)
- [GitHub — gastownhall/gastown](https://github.com/gastownhall/gastown)
- [Gas Town: What Kubernetes for AI Coding Agents Actually Looks Like (Cloud Native Now)](https://cloudnativenow.com/features/gas-town-what-kubernetes-for-ai-coding-agents-actually-looks-like/)
- [Gastown Quick Start Guide (DeepWiki)](https://deepwiki.com/steveyegge/gastown/1.2-quick-start-guide)
- [Cursor Background Agents — Official Docs](https://docs.cursor.com/en/background-agent)
- [Cursor 3 Agent-First Interface (InfoQ)](https://www.infoq.com/news/2026/04/cursor-3-agent-first-interface/)
- [Cloud Agent queue — Cursor Community Feature Request](https://forum.cursor.com/t/cloud-agent-queue/154653)
- [Best practices for coding with agents (Cursor Blog)](https://cursor.com/blog/agent-best-practices)
- [Claude Flow (Ruflo) v3.5 Complete Guide](https://pasqualepillitteri.it/en/news/774/claude-flow-ruflo-multi-agent-orchestration-guide)
- [GitHub — ruvnet/ruflo](https://github.com/ruvnet/ruflo)
- [Ruflo agent_spawn prompt delivery discussion #692](https://github.com/ruvnet/ruflo/discussions/692)
- [GitHub — tmuxinator/tmuxinator](https://github.com/tmuxinator/tmuxinator)
- [tmuxp YAML config (x-cmd)](https://www.x-cmd.com/install/tmuxp/)
- [gh pr create — Official Docs](https://cli.github.com/manual/gh_pr_create)
- [Linear — Creating Issues](https://linear.app/docs/creating-issues)
- [Linear — Select Issues (bulk actions)](https://linear.app/docs/select-issues)
- [Bulk action UX: 8 design guidelines (Eleken)](https://www.eleken.co/blog-posts/bulk-actions-ux)
- [Multi-Agent Orchestration: Running 10+ Claude Instances in Parallel (DEV Community)](https://dev.to/bredmond1019/multi-agent-orchestration-running-10-claude-instances-in-parallel-part-3-29da)
