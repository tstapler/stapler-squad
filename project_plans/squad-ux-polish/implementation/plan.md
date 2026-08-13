# Implementation Plan Summary: Squad UX Polish

**Phase**: 3 — Planning complete
**Date**: 2026-04-17
**Full plan**: `docs/tasks/squad-ux-polish.md`
**ADRs**: `project_plans/squad-ux-polish/decisions/ADR-001` through `ADR-004`

---

## Stories and Scope

| Story | Summary | Tasks | Size |
|-------|---------|-------|------|
| S1: Prompt at creation | CLAUDE.md injection + PromptStore + UI textarea + recent prompts | S1-1 through S1-5 | 5 tasks |
| S2: Batch creation | `BatchCreateSessions` RPC + bounded pool + batch UI tab | S2-1 through S2-3 | 3 tasks |
| S3: Review queue / one-shot | `RunOneShot` RPC + Create PR button + divergence warning | S3-1 through S3-3 | 3 tasks |
| S4: Project concept | ent `Project` entity + CRUD + project picker + GroupBy | S4-1 through S4-6 | 6 tasks |

**Total**: 17 tasks across 4 stories.

---

## Key Architectural Decisions

- **ADR-001**: Prompt delivery via CLAUDE.md injection (not `tmux send-keys`, not `--system-prompt`)
- **ADR-002**: `BatchCreateSessions` RPC with server-side bounded sequential worktree creation (max 3 concurrent, serialized per repo)
- **ADR-003**: `RunOneShot` RPC using `claude -p "<prompt>"` subprocess in worktree dir; server-side `gh` as fallback
- **ADR-004**: First-class ent `Project` entity with nullable FK on Session; tag convention rejected

---

## Critical Path

```
S4-1 (ent schema) ─────────────────────────────────────────────► S4-2 handlers
S1-1 (PromptStore)
     │
     ├── S1-2 ─ S2-1 ─ S3-1 ─ S4-2 proto (batch all proto edits, make generate-proto)
     │                                │
     │                                ├── S1-3 (CLAUDE.md injection)
     │                                ├── S1-4 (PromptHistory handlers)
     │                                ├── S2-2 (BatchCreateSessions handler)
     │                                ├── S3-2 (RunOneShot handler)
     │                                └── S4-3 (GroupByProject strategy)
     │
     └── UI tasks (S1-5, S2-3, S3-3, S4-4, S4-5, S4-6) — after backend handlers
```

---

## Pre-Implementation Checklist

Before starting a fresh implementation session, verify:

- [ ] `make build` passes on current branch
- [ ] `make test` passes on current branch
- [ ] Confirm `@.claude/session-prompt.md` imports resolve relative to worktree CLAUDE.md (not host repo CLAUDE.md)
- [ ] Confirm `ent.Schema.Create(ctx)` options in server startup do NOT include `WithDropColumn` or `WithDropIndex`
- [ ] Confirm `claude` binary is findable via `exec.LookPath` in the server process environment
- [ ] Read `session/instance.go:813` (`start()`) to understand the exact injection point for CLAUDE.md

---

## Out of Scope (Deferred to v2)

- AI-assisted code review before merge
- Batch-approve merge for multiple review queue items
- Template-based session creation (pre-fill form from named template)
- Session fork/clone (covered by existing `ForkSession` RPC; UI integration deferred)
- "Open in IntelliJ" button (Rich Drinkwater's separate PR)
- Prompt library scoped per-project (global workspace scope is sufficient for v1)
