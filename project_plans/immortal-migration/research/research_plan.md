# Research Plan: Process Manager Modularization

Created: 2026-04-29
Input: project_plans/immortal-migration/requirements.md

## Scope Statement

Evaluate whether and how stapler-squad can replace its hard Tmux dependency with a
Go-embeddable process manager (Immortal as primary candidate), via a config-driven
abstraction layer. Tmux stays as default; no forced migration.

---

## Subtopic 1: Stack (alternatives survey)

**Question**: What Go-native or Go-embeddable process supervisor libraries exist besides Immortal?

**Method**: Web search (Immortal is a daemon, not a library; alternatives may be)
**Search queries** (cap: 5):
1. "golang embedded process supervisor library"
2. "go process manager library suture s6 restarting"
3. "immortal process manager go library embed"
4. "go supervisor goroutine restart watchdog library"
5. "golang PTY process management library"

**Trade-off axes**: embeddability (library vs daemon), Go-native vs CGO, PTY support, restart policies, license, maintenance activity

**Output**: `research/findings-stack.md`

---

## Subtopic 2: Features (Immortal deep-dive)

**Question**: What does Immortal actually provide, and can it be embedded in a Go binary?

**Method**: code-archaeology on https://github.com/immortal/immortal — source is more authoritative than docs
**Supplement**: web search for "immortal process manager golang embed library" and "immortal daemon socket control"

**Trade-off axes**: library vs daemon architecture, PTY handling, restart policies, health checks, signal forwarding, control interface

**Output**: `research/findings-features.md`

---

## Subtopic 3: Architecture (current Tmux coupling audit)

**Question**: How tightly is stapler-squad's session logic coupled to Tmux-specific APIs? What would a `ProcessManager` interface need?

**Method**: Explore current codebase — session/, tmux/, server/services/ for Tmux-specific calls
**Key files to examine**: session/instance.go, session/tmux/, server/services/session_service.go

**Interface axes**: session lifecycle (create/stop/pause/resume), PTY streaming, supervision hooks, config surface

**Output**: `research/findings-architecture.md`

---

## Subtopic 4: Pitfalls

**Question**: What goes wrong with process supervision, PTY ownership, signal handling, and embedded supervisors in Go?

**Method**: Web search + training knowledge
**Search queries** (cap: 4):
1. "golang PTY process supervisor signal handling pitfalls"
2. "go zombie process embedded supervisor reaping"
3. "PTY ownership transfer process manager golang"
4. "immortal process manager known issues production"

**Trade-off axes**: zombie reaping, signal propagation (SIGTERM/SIGKILL/SIGCHLD), PTY lifetime, state machine complexity, test isolation

**Output**: `research/findings-pitfalls.md`
