# Validation Plan: pty-multiplexer

**Date**: 2026-06-13

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| FR-1a: Single PTY reader goroutine | `session/response_stream_test.go` | `TestResponseStream_OnlyOneGoroutineReadsFromPTY` | Unit | Verify multiplexer goroutine count stays at 1 after Subscribe; confirm no additional goroutines call `pty.Read` |
| FR-1a: Single PTY reader goroutine (error path) | `session/response_stream_test.go` | `TestResponseStream_DoubleStart_RejectsSecondReader` | Unit | `Start()` returns error on second call; existing `TestResponseStream_DoubleStart` already covers this — extend to assert goroutine count |
| FR-1b: Direct PTYAccess.Read forbidden in callers | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_DoesNotCallGetPTYReader_WhenControllerActive` | Integration | Mock `Instance` with a controller stub; assert `GetPTYReader()` is never invoked during streaming |
| FR-1c: Multiplexer starts / stops with PTY lifecycle | `session/response_stream_test.go` | `TestResponseStream_StartStop_LifecycleMatchesPTY` | Unit | Start stream, verify `IsStarted()==true`; close PTY pipe, verify `IsStarted()` transitions to false without explicit `Stop()` call |
| FR-2a: All bytes delivered to all subscribers | `session/response_stream_test.go` | `TestResponseStream_TwoSubscribersReceiveAllBytes` | Unit | os.Pipe PTY, two subscribers, write N distinct chunks; assert both received all N chunks in order (key regression test) |
| FR-2b: Slow subscriber does not block others | `session/response_stream_test.go` | `TestResponseStream_SlowSubscriberDoesNotBlockFastSubscriber` | Unit | Subscribe two consumers; intentionally do not drain slow consumer's channel past buffer capacity; write bursts; assert fast consumer still receives data within deadline |
| FR-2c: Overrun signal on subscriber lag | `session/response_stream_test.go` | `TestResponseStream_FullChannelDropsChunkWithWarning` | Unit | Use buffer size 1; fill subscriber channel; write additional data; assert chunk is dropped (not blocked), verify `broadcast()` returns without hanging |
| FR-2a: Fan-out byte equality check | `session/response_stream_test.go` | `TestResponseStream_TwoSubscribersReceiveIdenticalBytes` | Unit | Two subscribers; write fixed byte sequence; collect both streams; assert `bytes.Equal(stream1, stream2)` |
| FR-3a: Subscribe / Unsubscribe API | `session/response_stream_test.go` | `TestResponseStream_Subscribe_ReturnsChannelAndUnsubscribeClosesIt` | Unit | Existing `TestResponseStream_Unsubscribe` covers happy path; extend with channel-closed assertion after `Unsubscribe` |
| FR-3a: Concurrent subscribe safety | `session/response_stream_test.go` | `TestResponseStream_ConcurrentSubscribeUnsubscribe` | Unit | Existing test covers this; verify no data race under `-race` flag |
| FR-3b: Controller subscribes at startup | `session/claude_controller_test.go` | `TestClaudeController_SubscribesToResponseStream_OnStart` | Unit | Verify `cc.responseStream.GetSubscriberCount() >= 1` after `controller.Start()` |
| FR-3b: Controller unsubscribes on stop | `session/claude_controller_test.go` | `TestClaudeController_UnsubscribesFromResponseStream_OnStop` | Unit | Start controller; stop it; verify subscriber count returns to 0 |
| FR-3c: StreamTerminal subscribes per-connection | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_SubscribesToResponseStream_WhenControllerActive` | Integration | os.Pipe PTY + active ResponseStream; assert subscriber count increments when `StreamTerminal` goroutine starts |
| FR-3c: StreamTerminal unsubscribes on disconnect | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_UnsubscribesFromResponseStream_OnClientDisconnect` | Integration | Cancel stream context; assert subscriber count returns to 0 and no goroutine leak |
| FR-3d: Subscribe safe from any goroutine | `session/response_stream_test.go` | `TestResponseStream_ConcurrentSubscribeUnsubscribe` | Unit | Existing test; run under `go test -race` |
| FR-3e: GetPTYReader callers replaced | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_ReceivesFromResponseStream_WhenControllerActive` | Integration | Active controller with os.Pipe PTY; write to PTY; assert StreamTerminal output goroutine receives via channel, not raw `Read()` |
| FR-3e: Fallback when no controller | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_FallsBackToDirectRead_WhenNoController` | Integration | Instance with no controller; verify stream falls back to `GetPTYReader()` path and delivers bytes |
| FR-4a: Bytes written to circular buffer | `session/response_stream_test.go` | `TestResponseStream_StreamingWritesToBuffer` | Unit | Existing test covers this |
| FR-4a: Buffer write and subscriber delivery are atomic per chunk | `session/response_stream_test.go` | `TestResponseStream_BufferAndSubscriberReceiveSameBytes` | Unit | Write chunk; assert `buffer.GetAll()` contains the bytes AND subscriber channel delivers the same bytes |
| FR-4b: New subscriber receives circular buffer snapshot | `session/response_stream_test.go` | `TestResponseStream_LateSubscriber_ReceivesHistorySnapshot` | Unit | Start stream; write N bytes; subscribe second consumer; assert second consumer's first message contains history bytes (or history was sent before live deltas) |
| FR-4c: Preview reads from circular buffer unchanged | `session/instance_terminal_test.go` | `TestInstance_Preview_ReadsFromCircularBuffer_WithZeroOrMoreBrowserClients` | Integration | Populate circular buffer via ResponseStream; call `Preview()`; assert output matches buffer contents regardless of subscriber count |
| FR-5a: ClaudeController still receives all PTY bytes | `session/claude_controller_test.go` | `TestClaudeController_ReceivesAllPTYBytes_WhenStreamTerminalAlsoSubscribed` | Integration | os.Pipe PTY; controller + simulated StreamTerminal subscriber; write prompt bytes; assert controller's `statusBuffer` accumulates all bytes (neither consumer starves the other) |
| FR-5b: Sessions without controller fall back to CapturePaneContent | `session/instance_terminal_test.go` | `TestInstance_Preview_FallsBackToCapturePaneContent_WhenNoController` | Unit | Instance with no controller; verify `Preview()` delegates to `CapturePaneContent` mock |
| FR-5c: WriteToPTY unaffected | `session/instance_tmux_test.go` | `TestInstance_WriteToPTY_UnaffectedByMultiplexer` | Unit | Write to PTY via `WriteToPTY` while ResponseStream is active; assert write path never touches subscriber map |
| FR-5d: streamViaControlMode unaffected | `server/services/connectrpc_websocket_test.go` | `TestStreamViaControlMode_DoesNotUseGetPTYReader` | Unit | Assert `GetPTYReader` is never called in the control-mode code path (structural test / grep-based guard in CI is acceptable) |
| FR-6a: PTY replaced triggers multiplexer restart | `session/response_stream_test.go` | `TestResponseStream_PTYReplaced_SubscribersNotifiedAndReconnect` | Unit | Start stream; replace PTY via `ptyAccess.UpdatePTY(newPipe)`; assert old subscriber channel closes; new subscription on new PTY delivers bytes |
| FR-6a: Multiplexer restart after EIO | `session/response_stream_test.go` | `TestResponseStream_EIORestart_NewStreamDeliversBytes` | Unit | Close pipe to trigger EIO; verify `OnEOF` fires; start new ResponseStream on replacement PTY; assert new subscriber receives bytes |
| FR-6b: No goroutine leaks on PTY close | `session/response_stream_test.go` | `TestResponseStream_NoGoroutineLeaks_AfterStop` | Unit | `runtime.NumGoroutine()` before start; start + subscribe; stop; assert goroutine count returns to baseline (±1 for GC goroutines) |
| FR-6b: Subscriber channels drained and closed on EOF | `session/response_stream_test.go` | `TestResponseStream_PTYClosed` | Unit | Existing test — subscriber channel closes when PTY closes; verify no send-after-close panic |
| FR-6c: Multiple simultaneous browser clients receive same bytes | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_TwoSimultaneousClients_ReceiveIdenticalOutput` | Integration | Two concurrent `StreamTerminal` calls on same session; write to PTY; collect both output streams; assert byte equality |

## Adversarial Review Concerns → Test Mapping

| Concern | Test File | Test Name | Type | What it guards |
|---------|-----------|-----------|------|----------------|
| `chunk.Error` branch is dead code | `session/response_stream_test.go` | `TestResponseStream_ChunkError_IsNeverSetByStreamLoop` | Unit | Subscribe and drain 100 chunks written via os.Pipe; assert `chunk.Error == nil` for all received chunks — documents the invariant that channel close, not `Error` field, signals EOF |
| Send-after-close panic in WebSocket `done` channel | `server/services/terminal_websocket_test.go` | `TestTerminalWebSocketHandler_NoPanicOnConcurrentDisconnect` | Unit | Simulate two goroutines both trying to close `done` simultaneously; assert no panic (validates fix converts `done` to context-cancel or buffered-size-1 channel) |
| TOCTOU fallback race: controller starts after nil-check | `server/services/session_service_stream_terminal_test.go` | `TestStreamTerminal_NoDataLoss_WhenControllerStartsAfterNilCheck` | Integration | Start `StreamTerminal` with no controller; race a goroutine that starts a controller concurrently; write PTY bytes during the race window; assert all bytes are delivered to the WebSocket client (via fallback or subscription) |
| Lock order cc.mu → rs.mu undocumented | `session/claude_controller_test.go` | `TestClaudeController_LockOrder_NoDeadlock_UnderConcurrentSubscribeAndStatusCheck` | Unit | Concurrent goroutines: (a) `Subscribe` + `Unsubscribe` on `cc.responseStream`; (b) `cc.GetCurrentStatus()`; run with `-race -count=50`; assert no deadlock within timeout |

## Test Stack

- **Unit**: Go `testing` package + `testify/assert` + `testify/require`. PTY simulation via `os.Pipe()` (same pattern as `mockPTY()` in `session/pty_access_test.go`).
- **Integration**: Go `testing` package, `os.Pipe()` for PTY simulation, real `ResponseStream` + real `ClaudeController` stub wired to the stream, mock or in-process `ConnectRPC` stream for `StreamTerminal` tests. No external processes required.
- **Race detection**: All concurrent tests MUST pass with `go test -race`. The `TestResponseStream_TwoSubscribersReceiveAllBytes` test is the canonical race-detector target.

## New Test Files

| File | Package | Contents |
|------|---------|----------|
| `session/response_stream_test.go` | `session` | Extend existing file with FR-2a, FR-2b, FR-2c, FR-4b, FR-6a, FR-6b, adversarial concern 1 |
| `session/claude_controller_test.go` | `session` | Extend existing file with FR-3b, FR-5a, adversarial concern 4 |
| `server/services/session_service_stream_terminal_test.go` | `services` | **New file**: FR-1b, FR-3c, FR-3e, FR-6c, adversarial concerns 2 and 3 |
| `server/services/terminal_websocket_test.go` | `services` | Extend existing file with adversarial concern 2 (done-channel panic) |
| `session/instance_terminal_test.go` | `session` | Extend or create: FR-4c, FR-5b |

## Coverage Targets

- Unit test coverage: ≥80% line coverage on `session/response_stream.go` and `session/claude_controller.go` (response-stream methods)
- All public methods on `ResponseStream`: happy path + at least one error path each
- `StreamTerminal` integration: controller-active path AND no-controller fallback path both covered
- `TerminalWebSocketHandler`: concurrent-disconnect panic guard covered
- Race detection: `go test -race ./session/... ./server/services/...` must pass with zero data-race reports

## Priority Order

The following tests are highest priority and should be written first:

1. `TestResponseStream_TwoSubscribersReceiveAllBytes` — the core regression test; if this passes under `-race`, the fundamental fan-out bug is fixed
2. `TestStreamTerminal_ReceivesFromResponseStream_WhenControllerActive` — validates the broken caller is fixed
3. `TestStreamTerminal_FallsBackToDirectRead_WhenNoController` — validates backward compat for sessions without a controller
4. `TestStreamTerminal_UnsubscribesFromResponseStream_OnClientDisconnect` — goroutine leak guard
5. `TestTerminalWebSocketHandler_NoPanicOnConcurrentDisconnect` — pre-existing panic risk (adversarial review concern 2)

## Gaps and Notes

- **FR-4b (late-subscriber history snapshot)**: `ResponseStream` as currently implemented does not send a history snapshot on `Subscribe()` — it only delivers live chunks going forward. The plan refers to this as a capability (`FR-4b`), but the existing code defers it to `CapturePaneContent` in `StreamTerminal`. The test `TestResponseStream_LateSubscriber_ReceivesHistorySnapshot` should be written as a **pending/TODO test** that asserts the current behavior (no automatic replay) and documents the known gap. Do not block the PR on implementing replay — it is a follow-up.
- **FR-5d (streamViaControlMode)**: A compile-time structural guard (e.g., confirming `GetPTYReader` is not called in `connectrpc_websocket.go`) can be enforced with a grep in CI (`Makefile` target or GitHub Actions step) rather than a Go test. The test entry in this plan captures that requirement.
- **Acceptance criterion (5s idle detection)**: This is behavioral and timing-dependent; it is not directly unit-testable without a real tmux session. It will be verified by the existing E2E test suite and manual smoke test after landing.
