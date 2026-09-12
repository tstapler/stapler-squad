# BUG-109: The session-creation log line always said "creating tmux session", even for tymux-backed sessions [SEVERITY: Low]

**Status**: ✅ Fixed
**Discovered**: 2026-09-12, user report: "even when I flip the flag it doesn't seem like sessions
are being dispatched to tymux or at least I dont see logs" (referring to the `tymux` feature flag).

## Problem Description

`initTmuxSession` (`session/instance_tmux.go:505`) runs on every `Start()` path regardless of the
resolved `ProcessManager` backend — its own doc comment says as much ("Every Start() path calls
initTmuxSession() before starting the tmux session") — and unconditionally logged:

```go
log.Info("creating tmux session", "session", i.Title, "program", enrichedProgram)
```

For a tymux-backed session this is misleading, not wrong-but-harmless: the function constructs a
`tmux.TmuxSession` object and only actually wires it up if `i.processManager.(*TmuxBackend)`
type-asserts true (line 547); for a `*TymuxBackend` session that branch is skipped, the real
dispatch happens later in `TymuxBackend.Start`, but the log line already said "tmux" moments
earlier with no `tymux` mention anywhere near session creation.

## Investigation

Checked the live deployed instance's actual log
(`~/.stapler-squad/workspaces/d685c4b1a423cca3/logs/staplersquad.log`, the workspace with
`feature_flags.tymux = true` and four real `tymux_session_overrides` entries) — tymux dispatch is
in fact working extensively: `tymux: session ready` / `tymux: standing Attach stream opened` fire
for dozens of real sessions across many repos. But the *first* log line at session-creation time is
always the generic "creating tmux session" (`grep -c tymux` on that log returned 720 hits, none of
them at the exact moment a session starts) — confirming the reported symptom is this log line's
wording, not a functional dispatch failure. `docs/bugs/fixed/BUG-106-*.md`'s earlier investigation
had already flagged this exact line as "cosmetic" in passing; this is where it got root-caused.

## Fix

`session/instance_tmux.go`: renamed the message to `"creating session"` and added a `backend`
field, derived from a new `processManagerBackendLabel` helper
(`session/backend_observability.go`) that type-switches on the concrete `ProcessManager` —
`*TmuxBackend`/`*TymuxBackend`/`*NativeProcessManager` — rather than trusting the function's own
name or `Instance.Backend`'s possibly-stale/empty struct field. Also fixed a comment in
`session/instance_serialization.go` that quoted the old message text.

Coverage: `session/backend_observability_test.go`'s
`TestProcessManagerBackendLabel_MatchesConcreteType`.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md`,
`docs/bugs/fixed/BUG-108-no-latency-metrics-or-traces-to-compare-tymux-vs-tmux.md` (same
investigation thread — all three surfaced auditing the tymux rollout for default-on readiness).
