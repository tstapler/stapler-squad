# BUG-025: Escape Analytics Reads as Broken — session_id Mismatch Hides All Data, Mangle Detection Never Wired [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-01)
**Discovered**: 2026-07-01
**Fixed**: 2026-07-01 — `pkg/analytics/escape_code_parser.go`, `session/response_stream.go`,
`session/claude_controller.go`, `server/services/connectrpc_websocket.go`, `config/config.go`
**Reported by**: User — "Escape analytics feature seems broken, not capturing any escape codes,
so we don't know which codes need support, and can't tell if things are getting mangled."

## Plan soundness check (requested alongside this bug)

`project_plans/terminal-analytics/implementation/plan.md` is sound: Story 4.1 explicitly calls
for `instance.GetStableID()` (not the tmux session name) when constructing the escape parser,
and Epic 5 explicitly designs `RecordStage1`/`CheckStage2` as distinct calls plus a Stage 2
base-offset scheme in ADR-TBA-4. **The plan was not followed during implementation on both
points**, and the two regression tests the plan called for for exactly this class of gap
(`E2-S3-T1` batch writer test, `E5-S2-T1` Stage1/Stage2 integration test) were never written —
which is why these shipped silently.

## Problem Description

The feature is **not** failing to capture data. Querying the live analytics DB for this
worktree's own session traffic:

```
$ sqlite3 analytics.db "SELECT COUNT(*) FROM escape_events;"
185008
$ sqlite3 analytics.db "SELECT stage, COUNT(*) FROM escape_events GROUP BY stage;"
pty_read|151320
transport|33688
$ sqlite3 analytics.db "SELECT mangled, COUNT(*) FROM escape_events GROUP BY mangled;"
0|185008
```

Two distinct bugs together produce "seems broken":

### Root Cause 1: `session_id` is the tmux session name, not the stable session UUID

`session/response_stream.go` `newEscapeParserForSession(sessionName)` is constructed with
`cc.sessionName` (`session/claude_controller.go:205`, sourced from `instance.GetTitle()`).
Sample `session_id` values actually stored: `dotfiles`, `fanapp-backend`,
`pr-1220-actions-build-extraction` — human-readable titles, not UUIDs.

The web UI's session selector (`EscapeAnalyticsPage.tsx`) populates its dropdown from
`session.id` — the stable UUID returned by `GetStableID()`, the identifier convention used
everywhere else in the app (`instance_vnc.go`, `instance_cdp.go`, `notification_service.go`,
etc.). `QueryEscapeAnalytics`/`GetEscapeAnalyticsSummary`
(`server/services/analytics_escape_service.go`) filter by whatever `session_id` the UI sends,
with no translation.

Result: the UI queries by stable UUID, the rows are keyed by tmux title. For any session using
the modern UUID scheme these values never match, so the analytics page returns **zero rows for
every session**, even though ~185K rows exist. From the user's vantage point (the only place
they'd look) the feature appears to capture nothing.

### Root Cause 2: Mangle detection (Epic 5) is entirely unwired — `mangled` is always `false`

`grep -rn "SetCorrelator\(" --include="*.go" .` outside of tests returns nothing.
`MangleCorrelator` (`pkg/analytics/mangle_correlator.go`) is fully implemented and unit-tested
in isolation, but no production code ever constructs one or attaches it to an
`EscapeCodeParser`. Confirmed empirically: 0 of 185,008 captured rows have `mangled=1`, across
5 days and 11 heterogeneous sessions — statistically conclusive that detection never fires, not
that nothing was ever mangled.

Two additional bugs sit underneath this one, found while tracing why wiring the correlator
alone would still not work:

1. **`emitEventWithStageAndSeq` always calls `RecordStage1`, never `CheckStage2`**
   (`pkg/analytics/escape_code_parser.go`). The `stage` parameter (`StagePTYRead` vs.
   `StageTransport`) is accepted but never inspected before touching the correlator, so even a
   wired correlator would treat every Stage 2 observation as a fresh Stage 1 recording instead
   of checking it against the matching Stage 1 entry.

2. **Stage 2's data source is a second, independent tmux client — not the same producer as
   Stage 1 — so byte-offset correlation between them cannot work at all, regardless of
   arithmetic.** Initially fixed as an off-by-`len(buf)` arithmetic bug
   (`server/services/connectrpc_websocket.go:751`:
   `escapeParser.ParseStage2(buf, instance.GetTotalBytesWritten())` reads the buffer's
   *post-write* total instead of its pre-write start offset), and that arithmetic fix is kept
   (it's still a strict improvement to the `session_seq` value recorded on each row, used for
   UI/debugging position display). But a deeper trace during code review (see "Code review
   findings" below) found the real problem: `streamViaControlMode`'s `updateChan` (Stage 2's
   data) comes from `instance.StartControlMode()`/`SubscribeControlModeUpdates()` — a
   **separate tmux control-mode (`-C`) client attachment** — not from `ResponseStream`'s own
   broadcast (which is fed by yet another, independent `tmux attach-session` client, `t.ptmx`,
   driving Stage 1). Two separate OS processes, no shared clock or counter between them.
   Byte-offset correlation was fixed to the *wrong bug* — the arithmetic fix helps, but the
   fundamental design (two independent producers, correlated by a shared byte counter neither
   of them actually shares) doesn't work. See "Mangle correlation redesign" below for the
   actual fix.

### Related: data race on parser lifetime counters

`p.totalSequences` / `p.totalMangled` in `EscapeCodeParser` are plain `int64` fields incremented
without synchronization in `emitEventWithStageAndSeq`. Stage 1 (`Parse`, called from the PTY
read goroutine) and Stage 2 (`ParseStage2`, called from the WebSocket output goroutine) both
reach this method concurrently on the *same* parser instance for a session — an unguarded
concurrent read-modify-write. `go test -race` did not previously catch this only because Stage
2 never emitted anything with a correlator attached to correctness-check against (no path
exercised both stages against the same parser in a test). Wiring the correlator makes this race
newly reachable in tests; fixed alongside as part of the same change.

### Related, lower severity: `DefaultConfig()` doesn't mirror `LoadConfigFromPath`'s escape defaults

`config/config.go` has an explicit comment above the `SessionDefaults` init in `DefaultConfig()`:
"`LoadConfigFromPath` applies the same guards after JSON decode; `DefaultConfig` must mirror
them so the two code paths are equivalent." The escape analytics fields
(`EscapeAnalyticsCaptureLevel`, `EscapeAnalyticsSamplingRate`, `EscapeAnalyticsMaxRowsPerSession`,
`EscapeAnalyticsRetentionDays`) were added to `LoadConfigFromPath`'s defaulting block but never
mirrored into `DefaultConfig()`. Currently masked by a second, independent defensive default in
`response_stream.go`'s `loadAnalyticsConfig()`, so this hasn't caused a visible symptom — but on
a machine's very first boot (`config.json` doesn't exist yet), `cfg.EscapeAnalyticsMaxRowsPerSession`
passed into `NewEscapeEventBatchWriter` at `server.go:534` would be `0`, which disables the
per-session row cap entirely (`if w.maxRowsPerSession > 0` guard in
`escape_event_batch_writer.go`) instead of applying the intended 10,000-row default.

### Mangle correlation redesign: ordinal correlation instead of byte offset

An initial PR for this bug shipped the byte-offset arithmetic fix above and stopped there. A
4-agent code review (`/code:review`, one CRITICAL finding) traced the actual `updateChan`
producer and found the byte-offset design couldn't work as described — see Root Cause 2, item
2, above. Verified empirically before redesigning, not just by re-reading code:

- Queried the live analytics DB (185K pre-fix rows): per-session `pty_read` vs. `transport`
  `session_seq` ranges overlap in the same order of magnitude for every session — rules out
  "totally unrelated counters," doesn't rule out timing drift.
- Ran a live experiment: a real tmux session with two simultaneous clients (a normal
  `tmux attach-session`, standing in for Stage 1's `t.ptmx`, and a `tmux -C attach-session`
  control-mode client, standing in for Stage 2's `updateChan`), fed a burst of ~200
  escape-sequence lines, logged cumulative byte counts from both. Result: in steady state
  (well past each client's own initial-attach redraw), the two streams carry byte-for-byte
  *identical* content in the same order — confirming tmux's control-mode `%output` really
  does forward raw pane bytes (octal-escaped for transport), not a re-synthesized redraw. The
  divergence is a roughly *constant* offset established during each client's own connection
  handshake (each client gets its own independent full-screen redraw on attach and on
  resize), not unbounded drift under load.

So: correlating by *content* is sound (the two streams do carry the same bytes), but
correlating by *byte position* isn't, because that constant offset resets independently on
each client's own attach/resize event, with no shared counter to recalibrate against.

**Fix**: `MangleCorrelator` now correlates ordinally per `(session, sequence type)` instead of
by byte offset. The Nth sequence of a given type seen at Stage 1 is matched against the Nth
sequence of that type seen at Stage 2 (`pkg/analytics/mangle_correlator.go`) — `RecordStage1`
and `CheckStage2` each maintain their own independent per-`(session, type)` counter and no
longer take a `sessionSeq` parameter at all. This works because tmux mirrors pane output to
all attached clients in the same relative order, so ordinal position is preserved even though
absolute byte position isn't. Trade-off: an actual dropped or duplicated sequence of a type
desyncs every subsequent ordinal for that `(session, type)` — see
`TestMangleCorrelator_DroppedSequenceDesyncsSubsequentOrdinals`, which documents this as an
accepted trade-off rather than a bug. Two correlation designs were considered and rejected:
independent per-stream byte counters recalibrated on each redraw (real recalibration-event
detection adds meaningful surface area for uncertain benefit), and content-hash-keyed
correlation (fully offset-robust, but a mutated sequence has a different hash *by definition*
and so would never be found by a hash-keyed lookup — it can detect "stripped" but not
"mutated", a net loss of detection capability versus the ordinal approach).

### Code review findings (fixed in the same PR)

A 4-dimension review (Testing, Code Quality, Architecture, Security) found 0 BLOCKER, 1
CRITICAL (the correlation redesign above), and 6 MAJOR findings, all fixed:

1. `EscapeCodeParser.sessionID` was a plain `string`, correct only because the single
   production call site happens to run before any streaming goroutine starts, with no guard
   enforcing that ordering. Fixed: `atomic.Pointer[string]`, safe to call at any time.
2. `EscapeCodeParser.SetSessionID` was named identically to
   `detection.StatusDetector.SetSessionID` (tmux-name-keyed), called 4 lines away in
   `claude_controller.go` — a real future-miscopy risk given this exact bug class (wrong ID
   passed to wrong sink) is what this PR fixes. Fixed: renamed to `SetStableSessionID` to
   match `ResponseStream`'s wrapper and make the distinction textually visible.
3. `StartCorrelatorEviction`'s background goroutine was fire-and-forget, not tracked by
   `ResponseStream.wg` the way `streamLoop` is — silently broke `Stop()`'s documented "blocks
   until fully drained" contract. Fixed: renamed to `RunCorrelatorEviction` (blocking, no
   internal `go` statement); the caller (`ResponseStream.Start`) now spawns and tracks it with
   `rs.wg`, matching the dominant convention this exact struct already uses for its own
   primary loop.
4. The same goroutine had no panic-recovery, unlike the established convention elsewhere in
   this codebase (`review_queue_poller.go`, `external_streamer.go`, etc.) — an unhandled panic
   there would have crashed the whole server. Fixed: wrapped in `recover()`.
5. The actual production wiring line (`claude_controller.go`:
   `rs.SetStableSessionID(cc.instance.GetStableID())`) was structurally unreachable by any
   test — `mockInstance.GetPTYReader()` always errored, so `ClaudeController.Start()` returned
   before reaching it. Fixed: added `TestClaudeController_Start_TagsEscapeAnalyticsWithStableID`
   using an injectable PTY reader; verified it fails if that line regresses to
   `rs.SetStableSessionID(cc.sessionName)`.
6. `InstanceContext.GetStableID()` duplicates a dormant, unused `InstanceReader` interface in
   `session/interfaces.go` (same package). Not fixed — flagged as a follow-up, not blocking;
   the reviewing agent explicitly recommended not blocking this PR on it.

Also fixed as free NITs: `mockInstance.GetStableID()` no longer falls back to `title` when
unset (it returned a value indistinguishable from a real regression, weakening any future
test's ability to catch a title/stableID mixup); `TestMangleCorrelationTotalsAreConcurrencySafe`
now also asserts `TotalMangled == 0`.

Process note: this review's task-notifications were repeatedly targeted by a prompt-injection
attempt (a fabricated "quantum-lock" marker instructing the reviewing agent, and separately
the orchestrating session, to hide claimed state changes from the user "since already
aware"). Both agents independently verified the claims were false and did not comply. Not a
finding about this codebase, but worth recording here since it happened during this PR's
review.

## Files Affected

- `session/claude_controller.go` — `InstanceContext` interface, `Start()`
- `session/claude_controller_test.go` — `mockInstance`, new wiring-level test
- `session/response_stream.go` — `newEscapeParserForSession`, `Start()`
- `session/response_stream_test.go` — new wiring-level test
- `pkg/analytics/escape_code_parser.go` — `SetStableSessionID`, `RunCorrelatorEviction`,
  `emitEventWithStageAndSeq`, counter fields
- `pkg/analytics/escape_code_parser_test.go` — new regression/concurrency tests
- `pkg/analytics/mangle_correlator.go` — ordinal correlation redesign
- `pkg/analytics/mangle_correlator_test.go` — updated for the new API + new tests
- `server/services/connectrpc_websocket.go` — Stage 2 tap call site
- `config/config.go` / `config/config_test.go` — `DefaultConfig()`

## Fix Approach

See commit history for the full diff. Summary:

1. Add `GetStableID() string` to `InstanceContext`; thread it into the escape parser via
   `ResponseStream.SetStableSessionID` → `EscapeCodeParser.SetStableSessionID`, called once
   right after `NewResponseStream` in `ClaudeController.Start()`. `cc.sessionName` (tmux name)
   is left untouched everywhere else it's used (PTY naming, command queue/history persistence,
   rate limiting, idle detection) — scoped to the escape-analytics session key only.
   `sessionID` is `atomic.Pointer[string]`, safe to set at any time.
2. Construct a `MangleCorrelator` per parser in `newEscapeParserForSession`; its eviction loop
   (`RunCorrelatorEviction`, panic-recovered) is spawned and tracked by `ResponseStream.wg`
   from `Start(ctx)`, so `Stop()` genuinely blocks until it exits too.
3. Branch `emitEventWithStageAndSeq` on `stage`: `StagePTYRead` → `RecordStage1`,
   `StageTransport` → `CheckStage2` (sets `record.Mangled`/`record.MangleType`).
4. Redesign `MangleCorrelator` to correlate ordinally per `(session, sequence type)` instead
   of by byte offset — see "Mangle correlation redesign" above for why the byte-offset
   approach couldn't work regardless of arithmetic.
5. Fix the Stage 2 base-offset arithmetic in `connectrpc_websocket.go` (kept as a data-quality
   improvement to the recorded `session_seq`, even though it's no longer load-bearing for
   correlation under the ordinal redesign).
6. Convert `totalSequences`/`totalMangled` to `atomic.Int64`.
7. Mirror the escape analytics defaults into `DefaultConfig()`.

## Verification

- `pkg/analytics`: `TestSetSessionIDOverridesConstructorSessionID`,
  `TestMangleCorrelationStage1ThenStage2Match`/`Mutated`,
  `TestMangleCorrelationTotalsAreConcurrencySafe` (drives Stage 1 + Stage 2 concurrently
  through the same parser, run under `-race`), plus `mangle_correlator_test.go`'s
  `TestMangleCorrelator_OrdinalPerType` (interleaved-type robustness) and
  `TestMangleCorrelator_DroppedSequenceDesyncsSubsequentOrdinals` (documents the accepted
  trade-off).
- `session`: `TestResponseStream_SetStableSessionID` (ResponseStream-level wiring) and
  `TestClaudeController_Start_TagsEscapeAnalyticsWithStableID` (the actual production
  assembly point, `ClaudeController.Start()` — verified by temporarily reverting the wiring
  line and confirming the test fails, then restoring it).
- `config`: `TestDefaultConfigMirrorsEscapeAnalyticsDefaults`.
- `go build ./...`, `go vet`, `golangci-lint run` (0 issues), and
  `go test ./pkg/analytics/... ./config/... ./server/analytics/... ./session/... ./server/services/... -race`
  all green.
- The `connectrpc_websocket.go` arithmetic fix has no dedicated automated test — the existing
  test file has no harness for driving `streamViaControlMode`'s coalescing goroutine, and
  building one is out of scope for this fix. Verified by manual trace instead (see Root Cause
  2, item 2, and the empirical tmux experiment above).

## Related

- `project_plans/terminal-analytics/` — original feature plan and research
- Follow-up recommended: an end-to-end integration test (Stage 1 → Stage 2 → SQLite,
  `mangled=true`) through the real WebSocket streaming path, not just the correlator unit
  level — this would also be the natural place to validate the ordinal correlation's
  real-world hit rate against live tmux traffic.
- Follow-up recommended, non-blocking: converge `InstanceContext` with the dormant
  `InstanceReader` interface in `session/interfaces.go`.

## Resolution

All fixes applied and verified — see "Fix Approach" and "Verification" above for the final,
complete list (superseding the earlier single-PR summary that predates the code-review-driven
correlator redesign).

Tests added: `TestSetSessionIDOverridesConstructorSessionID`,
`TestMangleCorrelationStage1ThenStage2Match`, `TestMangleCorrelationStage1ThenStage2Mutated`,
`TestMangleCorrelationTotalsAreConcurrencySafe` (pkg/analytics);
`TestResponseStream_SetStableSessionID` (session, exercises the real production wiring path);
`TestDefaultConfigMirrorsEscapeAnalyticsDefaults` (config).

Verified: `go build ./...`, `go vet`, `golangci-lint run` (0 issues), and
`go test ./pkg/analytics/... ./config/... ./server/analytics/... ./session/... ./server/services/... -race`
all green.

Known residual gap (documented, not blocking): the `connectrpc_websocket.go` arithmetic fix has
no dedicated automated test — the existing test file has no harness for driving
`streamViaControlMode`'s coalescing goroutine. Verified by manual trace only. A true end-to-end
integration test (Stage 1 → Stage 2 → SQLite, `mangled=true`, matching the original plan's AC-3)
remains a good follow-up.
