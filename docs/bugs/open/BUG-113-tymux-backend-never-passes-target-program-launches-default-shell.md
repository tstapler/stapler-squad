# BUG-113: tymux backend never passes the target program — every session launches `$SHELL` instead [SEVERITY: High]

**Status**: 🐛 Open
**Discovered**: 2026-09-16, user report: "our implementation of the stapler-squad tymux integration doesn't start the right target program", plus a related report that streaming logs don't work.

## Problem Description

`session/tymux/session.go`'s `tymuxGRPCSession.Start` builds its `CreateSessionRequest` with only
`Name` and `Cwd` set:

```go
resp, err := s.transport.CreateSession(context.Background(), connect.NewRequest(&v1.CreateSessionRequest{
	Name: dir,
	Cwd:  dir,
}))
```

The proto itself has a third field the tymux daemon uses for exactly this purpose:

```go
// github.com/tstapler/tymux/clients/go/gen/tymux/v1
type CreateSessionRequest struct {
	Name    string
	Command string // defaults to $SHELL if empty
	Cwd     string
}
```

`Command` is never set, anywhere. `tymuxGRPCSession` has no `program` field at all (checked its
struct definition, `session/tymux/session.go:33-`), and its constructor,
`NewTymuxGRPCSession(transport rpcTransport) TymuxManager`, takes no program parameter either —
compare `NewTmuxSession(name string, program string, opts ...TmuxSessionOption)` in the sibling
`session/tmux` package, which does. There is no code path today by which a tymux-backed session's
configured program (e.g. `claude --resume <uuid> ...`) can reach tymuxd. Every tymux-backed session
launches tymuxd's default `$SHELL` instead, regardless of what the session was actually configured
to run.

## Reproduction Steps

1. Create a session with the tymux backend enabled (`process_manager_backend = "tymux"` or
   equivalent) and a non-shell `Program` (e.g. `claude`).
2. Attach to the session.
3. Expected: the configured program (`claude ...`) is running in the pane.
4. Actual: the pane runs `$SHELL` (an interactive shell), never the configured program.

Not independently reproduced end-to-end in this session — the `Command` field being unset is
confirmed by direct source inspection (both the call site above and the proto definition), which by
itself is sufficient to guarantee the reported symptom, but no live tymuxd session was attached to
in the course of filing this bug.

## Root Cause

`tymuxGRPCSession.Start` (and its sibling `RestoreWithWorkDir`/`recreate` paths, if they build their
own `CreateSessionRequest`s — worth checking when this is fixed) never populates `Command`. The
value needed (the session's `Program`) is available at the call site that constructs the tymux
backend (`session/backend_tymux.go`'s `NewTymuxBackend`/whatever wires up `NewTymuxGRPCSession`),
but is never threaded through the constructor or `Start`'s own signature down to this RPC call.

## Files Likely Affected

- `session/tymux/session.go` — `Start`, `NewTymuxGRPCSession`, and `tymuxGRPCSession`'s struct
  definition (needs a `program` field, set from the constructor, used in `Start`'s
  `CreateSessionRequest`)
- `session/backend_tymux.go` — wherever `NewTymuxGRPCSession` is called; needs to pass the
  program through, mirroring how `NewTmuxSession(name, program, ...)` already does for the tmux
  backend
- Any `RestoreWithWorkDir`/session-recreate path in `session/tymux/session.go` that might also
  build a `CreateSessionRequest` on a cold-restore, if one exists — needs the same fix to avoid
  silently reverting to `$SHELL` after a tymuxd restart

## Fix Approach

Add a `program string` field to `tymuxGRPCSession`, accept it as a parameter on
`NewTymuxGRPCSession` (matching `NewTmuxSession`'s existing signature shape in the sibling
package), and set `Command: s.program` in `Start`'s `CreateSessionRequest`. Update
`session/backend_tymux.go`'s construction call site to pass the instance's actual `Program` value
through. Add a regression test asserting `CreateSessionRequest.Command` matches the configured
program (a fake/mock `rpcTransport` that captures the request, mirroring existing `session/tymux`
test patterns) — this bug's whole failure mode is invisible without one, since nothing currently
asserts on the request's `Command` field at all.

## Related Issue (reported alongside this one, not yet root-caused)

User also reported "the streaming logs don't work" for the tymux integration in the same
conversation. Not investigated here — `session/tymux/stream.go`'s `openStandingStream`/
`readAttachLoop`/`ReconnectLoop` are the likely area (585 lines, handles the standing Attach-stream
that carries pane output), but no failing repro or log evidence was captured for this half of the
report. Needs its own investigation before a root cause can be claimed; flagging here only so it
isn't lost, not as a duplicate-effort split.

## Related Bugs

- BUG-108: no latency/tracing instrumentation exists to compare tymux against tmux — same
  underlying gap in tymux observability/testing rigor that let this ship unnoticed.
- BUG-102: an existing tymux stream-generation flaky test, unrelated cause but same package
  (`session/tymux/stream.go`) as the unconfirmed streaming-logs report above.
