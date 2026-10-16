# BUG-108: No latency metrics or traces exist to compare the tymux backend against tmux [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-09-12, follow-up question during the tymux rollout-readiness review (BUG-106)
asking what evidence exists that tymux performs acceptably, not just correctly.

## Problem Description

There was no way to see, from metrics or traces, whether the tymux backend is faster, slower, or
equivalent to tmux for any real operation (session creation, attach, first-input latency, command
round-trip). Concretely, per-package instrument inventory at the time of discovery:

- `server/services/session_creation_metrics.go`'s `session.creation.duration_ms` histogram was the
  only cross-backend latency metric that existed, and it's labeled only by `outcome`
  (success/failure) — not by backend. A tmux-backed and a tymux-backed session's creation time land
  in the same bucket, indistinguishable.
- `session/tymux/stream.go`'s `tymux_attach_stream_reconnects_total` counter is tymux-only, and
  measures reconnect *frequency*, not latency — no tmux equivalent exists to compare against
  either.
- `session/lifecycle/observability.go`'s `session_lifecycle_active_generations`/`_ends_total` are
  generic leak-detection instruments keyed by `subsystem` (e.g. `tmux_control_mode` vs
  `tymux_stream`) — different subsystems doing different work, not a latency comparison.
- Zero OTel tracing spans existed anywhere in `session/tymux/` or `session/backend_tymux.go`.

Compare this to the `native_git_worktree`/`native_git_merge` rollout
(`session/git/native_rollout.go`), which was built with exactly this question in mind: a
`git_operation_duration_ms` histogram labeled `operation × implementation` ("native"/"legacy") plus
an OTel span per dispatch point via `withOperationSpan`, explicitly documented as existing to prove
the subprocess-elimination is a real perf win (Epic 4.4, Task 4.4.2a).

## Why this matters for the rollout decision

BUG-106 established that tymux *works* (a session reaches `SESSION_STATUS_ACTIVE`) once the daemon
can actually start. It said nothing about whether tymux-backed sessions are as responsive as
tmux-backed ones under real use — attach latency, keystroke-to-echo latency, and reconnect time are
all plausible regressions a gRPC-mediated daemon could introduce that a pure functional test
wouldn't catch, and which matter directly to "is this good enough to make everyone's default
session backend."

## Fix

Mirrored `session/git/native_rollout.go`'s pattern in a new file,
`session/backend_observability.go` (package `session`, since that's where `TmuxBackend` and
`TymuxBackend` both live side by side, unlike `session/git`'s single-package rollout):

1. `session_backend_operation_duration_ms`, a histogram labeled `backend` (`tmux`/`tymux`) ×
   `operation`, plus a Tempo span per operation (`withBackendOperationSpan`) — wired into both
   backends' `Start`, `RestoreWithWorkDir`, and `Attach` (`session.backend.start`/`.restore`/
   `.attach`).
2. `TymuxBackend.ensureDaemonReady` gets its own nested span/metric observation
   (`session.backend.tymux_daemon_ready`, `backend="tymux"` only) inside `Start`/
   `RestoreWithWorkDir`'s outer span — isolates daemon cold-start latency (what BUG-106's fix
   directly affects) from the rest of `Start`'s work, so a slow `session.backend.start` trace can be
   attributed to "daemon wasn't ready yet" vs. "the session itself was slow to start" at a glance.
3. `backend` is a distinct `backendLabel` type, not a second bare `string` parameter alongside `op`
   — `withBackendOperationSpan`'s two-string-parameter shape was flagged by this repo's
   primitive-obsession check during review; fixed by typing `backend` instead of leaving it a
   swappable-with-`op` bare string.

Verified live end-to-end (not just unit tests): built a manual instance, created one tmux-backed
and one tymux-backed session against it (with `tymux`'s global override flipped on for the second),
both reached `SESSION_STATUS_ACTIVE` — confirms the new span/metric wrapping doesn't regress either
backend's real startup path.

Coverage: `session/backend_observability_test.go` — span name/attributes on success and error,
histogram data-point count increments on a `withBackendOperationSpan` call, and two end-to-end
checks (`TestTmuxBackend_Start_RecordsSpanWithTmuxBackendLabel`,
`TestTymuxBackend_Start_RecordsSpanWithTymuxBackendLabel`) that the real `TmuxBackend`/`TymuxBackend`
wrappers — not just the helper in isolation — carry the correct `backend` label, guarding against
the easy mistake of copy-pasting one backend's constant into the other's wrapper. Also documented
in `docs/how-to/enable-opentelemetry.md`'s "Instrumented Operations" list.

## Out of scope

A Grafana panel/dashboard comparing the `backend` attribute's `tmux` vs `tymux` series side by side
— this repo has no checked-in dashboard JSON for any existing metric (Grafana is managed
externally); building the actual comparison view is a follow-up for whoever operates that instance,
now that the underlying metric exists to build it from.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md` (functional
fix this gap was found alongside), `session/git/native_rollout.go` (the pattern mirrored),
`.claude/skills/rollout-readiness-review/SKILL.md` (updated to check for this class of gap in
future rollout reviews).
