# Research Plan: TMux Session Robustness & API Controllability

Date: 2026-04-16
Input: project_plans/tmux-session-robustness/requirements.md

## Subtopics

### 1. Stack (findings-stack.md)
**Question:** Should we keep the current `tmux attach -CC` PTY approach, implement the tmux
control mode protocol directly over a socket, or replace tmux with a different process manager?

Search strategy: Training knowledge first; web searches for tmux control mode protocol spec,
Go tmux client libraries, supervisord/s6 Go bindings, recent production tmux-Go integrations.

Search cap: 4 searches
Trade-off axes: protocol complexity, event fidelity (exit codes, pane lifecycle), restart
survival, implementation cost, operational familiarity

---

### 2. Features (findings-features.md)
**Question:** How do other terminal multiplexers and agent managers handle session exit
detection, reconnection after restart, and lifecycle event delivery?

Search strategy: Survey Zellij (Rust, modern), tmuxinator (Ruby, popular), Wezterm multiplexer
API, typical Go daemon/supervisor patterns for PTY management.

Search cap: 4 searches
Trade-off axes: exit event delivery model, reconnect-after-restart capability, API surface
cleanliness, language/ecosystem fit

---

### 3. Architecture (findings-architecture.md)
**Question:** What are the established design patterns for session lifecycle management in Go —
event hooks, zombie detection, clean controller API surface?

Search strategy: Training knowledge (Go patterns well-established), web searches for
"Go PTY session lifecycle management", "tmux Go wrapper event driven", observer/hook patterns
in Go process supervision.

Search cap: 3 searches
Trade-off axes: observer pattern vs. polling, interface design (narrow vs. wide), goroutine
lifecycle safety, testability

---

### 4. Pitfalls (findings-pitfalls.md)
**Question:** What are the known failure modes when adding exit detection and zombie reconciliation
to a running PTY/tmux system in Go — race conditions, double-close, goroutine leaks?

Search strategy: Training knowledge + targeted searches for "Go PTY close race condition",
"tmux session zombie detection", production post-mortems on tmux Go wrappers.

Search cap: 3 searches
Trade-off axes: N/A — this is a risk catalogue, not a comparison

## Parallel Execution Plan

Spawn all 4 subagents simultaneously. Each writes to its own findings file.
Parent runs pending web searches after all complete.
Synthesize into research/synthesis.md.

## Success Criteria for Each Findings File

- [ ] All 5 research output questions answered (what exists, comparison, failure modes, adoption cost, recommendation)
- [ ] Concrete recommendation, not a survey
- [ ] Pending web searches listed if web access unavailable
