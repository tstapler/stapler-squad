# ADR-001: Initial Prompt Delivery Mechanism

**Status**: Accepted
**Date**: 2026-04-17
**Deciders**: Tyler Stapler

---

## Context

Story 1 requires an `InitialPrompt` field on `CreateSession`. When set, the prompt must be delivered to Claude Code at startup without race conditions or message truncation. Three candidate mechanisms exist.

The existing codebase already uses `i.Prompt` in `instance.go` line 1498–1499, delivered by appending it as a quoted CLI argument: `program = fmt.Sprintf("%s %q", program, i.Prompt)`. This means the current implementation does the append on the command line. The prompt value is exposed via `ps aux` and is size-constrained by shell argument limits.

Research confirmed:
- `tmux send-keys` has a 255-byte PTY path limit and a confirmed race condition (session must be at interactive prompt before keys are sent)
- `--system-prompt` CLI flag exposes content in the process list
- Claude Code has no `--init-prompt` flag
- CLAUDE.md injection uses Claude's own context loading mechanism; it's how per-session context already flows in via `GetClaudeConfig` RPC

---

## Decision

For prompts delivered at **interactive session startup** (non-one-shot): inject the prompt by writing it to `<worktree>/.claude/session-prompt.md` and appending an `@.claude/session-prompt.md` import line to the worktree's CLAUDE.md before the tmux session starts.

For **one-shot sessions** (`OneShot: true`): launch with `claude -p "<prompt>"` instead of interactive mode. The prompt is passed as a CLI argument to the one-shot invocation only; it is not appended to CLAUDE.md.

The current CLI-append mechanism (`fmt.Sprintf("%s %q", program, i.Prompt)`) is retained for backwards compatibility with short prompts but is superseded by CLAUDE.md injection for the `InitialPrompt` field from the new `CreateSession` field.

---

## Options Considered

### Option A: tmux send-keys (rejected)

Send the prompt as keystrokes after the session starts.

**Rejected because**:
- Confirmed race condition: Claude must be at its interactive prompt before keys arrive; no reliable readiness signal
- 255-byte limit on PTY path truncates long prompts silently
- Difficult to test deterministically

### Option B: --system-prompt CLI flag (rejected)

Pass `--system-prompt "text"` to the Claude process at startup.

**Rejected because**:
- Exposes prompt content in `ps aux` output — visible to any user on the machine
- Not appropriate for prompts containing sensitive task descriptions or API keys referenced by name
- Already rejected by the research synthesis

### Option C: CLAUDE.md injection (accepted)

Before starting the tmux session, write prompt to `<worktree>/.claude/session-prompt.md` and add `@.claude/session-prompt.md` to the worktree's CLAUDE.md.

**Accepted because**:
- Uses Claude Code's own context loading — Claude reads CLAUDE.md before its first interaction
- No process-list exposure (file write, not CLI arg)
- No size limit beyond filesystem
- `@path` imports in CLAUDE.md are resolved relative to the file location; worktree CLAUDE.md resolves relative to worktree root
- `session-prompt.md` can be git-ignored on worktree creation (add to `.gitignore`)
- Clean separation: prompt file is separate from project CLAUDE.md content

---

## Consequences

**Positive**:
- Prompt delivered reliably before Claude's first interaction
- No race condition or size limit
- Fully testable: inject prompt, verify CLAUDE.md contents before start()

**Negative / Accepted**:
- Prompt content ends up in a file in the worktree (`.claude/session-prompt.md`); visible to anyone with filesystem access
- CLAUDE.md modification means users viewing CLAUDE.md will see the import line; it is clearly labeled
- One extra file write per session with InitialPrompt set

**Implementation notes**:
- Inject in `setupFirstTimeWorktree()` path, after worktree is created but before `tmuxManager.Start()` is called
- Add `session-prompt.md` to `.gitignore` on worktree creation
- If worktree CLAUDE.md does not exist, create it
- The `@.claude/session-prompt.md` line should be appended at the end of CLAUDE.md, after any existing content
- Clean up `session-prompt.md` on `Destroy()` (worktree cleanup)
