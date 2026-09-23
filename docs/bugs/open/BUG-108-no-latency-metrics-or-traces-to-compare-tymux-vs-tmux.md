# BUG-108: No latency metrics or traces exist to compare the tymux backend against tmux [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-12, follow-up question during the tymux rollout-readiness review (BUG-106)
asking what evidence exists that tymux performs acceptably, not just correctly.

## Problem Description

There is no way today to see, from metrics or traces, whether the tymux backend is faster, slower,
or equivalent to tmux for any real operation (session creation, attach, first-input latency,
command round-trip). Concretely, per-package instrument inventory:

- `server/services/session_creation_metrics.go`'s `session.creation.duration_ms` histogram is the
  only cross-backend latency metric that exists, and it's labeled only by `outcome`
  (success/failure) — not by backend. A tmux-backed and a tymux-backed session's creation time land
  in the same bucket, indistinguishable.
- `session/tymux/stream.go`'s `tymux_attach_stream_reconnects_total` counter is tymux-only, and
  measures reconnect *frequency*, not latency — no tmux equivalent exists to compare against
  either.
- `session/lifecycle/observability.go`'s `session_lifecycle_active_generations`/`_ends_total` are
  generic leak-detection instruments keyed by `subsystem` (e.g. `tmux_control_mode` vs
  `tymux_stream`) — different subsystems doing different work, not a latency comparison.
- Zero OTel tracing spans exist anywhere in `session/tymux/` or `session/backend_tymux.go` (checked:
  no `tracer.Start`/`otel.Tracer` call in the package).

Compare this to the `native_git_worktree`/`native_git_merge` rollout
(`session/git/native_rollout.go`), which was built with exactly this question in mind: a
`git_operation_duration_ms` histogram labeled `operation × implementation` ("native"/"legacy") plus
an OTel span per dispatch point via `withOperationSpan`, explicitly documented as existing to prove
the subprocess-elimination is a real perf win (Epic 4.4, Task 4.4.2a). That rollout can show a
side-by-side latency comparison today; tymux cannot show one at all.

## Why this matters for the rollout decision

BUG-106 established that tymux *works* (a session reaches `SESSION_STATUS_ACTIVE`) once the daemon
can actually start. It says nothing about whether tymux-backed sessions are as responsive as
tmux-backed ones under real use — attach latency, keystroke-to-echo latency, and reconnect time are
all plausible regressions a gRPC-mediated daemon could introduce that a pure functional test
wouldn't catch, and which matter directly to "is this good enough to make everyone's default
session backend."

## Fix Approach

Mirror `session/git/native_rollout.go`'s pattern, scoped to the operations that matter most for
perceived responsiveness:

1. A `tymux_operation_duration_ms` (or extend `session.creation.duration_ms` with a `backend`
   attribute — cheaper, reuses an existing panel) histogram labeled `operation × backend`
   (`tmux`/`tymux`), recorded at minimum around: session creation/first-attach, and
   `TymuxBackend.Start`'s `ensureDaemonReady` call (isolates daemon-cold-start latency, which
   BUG-106's fix directly affects, from steady-state operation latency).
2. An OTel span per `TymuxBackend` method that proxies to the daemon (`Attach`, `SendKeys`, resize),
   with `backend="tymux"` as a span attribute, so a slow trace is traceable to the RPC hop
   specifically rather than looking identical to a slow tmux command in Tempo.
3. A Grafana panel (or dashboard section) comparing the `backend` attribute's `tmux` vs `tymux`
   series side by side — the actual deliverable an operator would look at before flipping the
   global default.

Not done in this pass: this is new instrumentation work, not a bug fix to existing behavior, and
warrants its own scoped implementation (plausibly via `/sdd:quick` or `/sdd:fix-bug`) rather than
folding into BUG-106's fix.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md` (functional
fix this gap was found alongside), `session/git/native_rollout.go` (the pattern to mirror),
`.claude/skills/rollout-readiness-review/SKILL.md` (updated to check for this class of gap in
future rollout reviews).
