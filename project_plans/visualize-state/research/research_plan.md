# Research Plan: Session State Visibility & Triage UX

Created: 2026-04-14
Input: project_plans/visualize-state/requirements.md

## Subtopics

### 1. Stack (`findings-stack.md`)
**Question**: What are the best options for rendering terminal snapshots (ANSI output) in a React web UI, and how should snapshot data be pushed from the backend?

Search strategy:
- ANSI rendering libraries for React/browser (xterm.js, react-terminal-ui, ansi-to-html, xterm-for-react)
- Polling vs. SSE vs. WebSocket vs. ConnectRPC streaming for terminal snapshot delivery
- Terminal snapshot formats (raw ANSI bytes, VT100 state, rendered HTML)

Search cap: 4 searches
Key axes: bundle size, rendering fidelity, scroll performance, maintenance activity, integration cost with existing ConnectRPC streaming

---

### 2. Features (`findings-features.md`)
**Question**: How do comparable tools handle multi-session triage and non-interrupting notification/approval flows?

Search strategy:
- tmux web UIs (ttyd, wetty, gotty) - session list / status display patterns
- CI dashboard UIs (GitHub Actions, Buildkite, CircleCI) - parallel job status cards
- Terminal multiplexers and session managers - how they surface "needs attention" state
- Non-interrupting approval UIs: Slack approval workflows, GitHub PR review queue stability patterns

Search cap: 4 searches
Key axes: triage speed, approval UX pattern (modal vs. sidebar vs. badge), status update stability, information density

---

### 3. Architecture (`findings-architecture.md`)
**Question**: What design patterns best support (a) wiring Go backend pattern-detected session state to a React frontend, and (b) a non-interrupting hook approval flow?

Search strategy:
- React patterns for non-interrupting notifications: toast queues, sidebars, floating panels
- ConnectRPC server-sent streaming for status push
- Go backend terminal output watching patterns (regex on scrollback, state machine)
- Optimistic UI patterns for approval flows

Search cap: 4 searches
Key axes: latency of status propagation, UI stability (no layout jumps), approval interaction model, code complexity

---

### 4. Pitfalls (`findings-pitfalls.md`)
**Question**: What are the known failure modes for this class of feature?

Search strategy:
- ANSI/VT100 rendering bugs in browser (color codes, cursor control, partial sequences)
- React list re-render instability (key thrashing, scroll position loss)
- ConnectRPC/WebSocket reconnect edge cases and status staleness
- Race conditions in Go session status detection (false positives/negatives for "waiting" state)

Search cap: 4 searches
Key axes: severity, frequency in production, mitigation availability, detection difficulty

---

## Output Files

- `research/findings-stack.md`
- `research/findings-features.md`
- `research/findings-architecture.md`
- `research/findings-pitfalls.md`
- `research/synthesis.md` (parent synthesizes after all 4 complete)
