# BUG-085: `TestPluginWatcher_should_collapseBurstIntoOneReload_When_sameFileWrittenRepeatedly` is flaky under CI load [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-21, in CI for PR #585 (`backlog/stapler-squad-fix-list-backlog-items-allowed-transitions`), a fix scoped entirely to `server/services/backlog_service.go`, its test, and `web-app/src/lib/store/backlogItemsSlice.ts` plus tests — none of which touch `session/detection`.
**Impact**: Intermittent CI red on `go test ./session/detection/...`, unrelated to the PR that happens to trigger it. Run: https://github.com/tstapler/stapler-squad/actions/runs/32518457560/job/96885837222

## Problem Description

`go test ./session/detection/...` failed with:

```
--- FAIL: TestPluginWatcher_should_collapseBurstIntoOneReload_When_sameFileWrittenRepeatedly (0.67s)
    plugin_watcher_test.go:148:
        Error Trace: session/detection/plugin_watcher_test.go:148
        Error:       Condition satisfied
        Test:        TestPluginWatcher_should_collapseBurstIntoOneReload_When_sameFileWrittenRepeatedly
        Messages:    burst of rapid writes triggered more than one rebuild
```

The test (`session/detection/plugin_watcher_test.go:127-155`) writes the same plugin TOML file 10 times, sleeping `5 * time.Millisecond` between writes, intending to stay "well inside" `pluginReloadDebounce` (`session/detection/plugin_watcher.go:20`, 200ms). It then asserts via `require.Never` that no second rebuild happens in the following 500ms.

The failure means a second rebuild *did* land — i.e., the debounce window was crossed at least once during the burst. `time.Sleep(5 * time.Millisecond)` only guarantees the goroutine won't wake *before* 5ms elapse; under a contended CI runner (this job runs the full `-race` suite plus tmux/PTY-backed tests concurrently), actual scheduling delay for any one of the 10 write→sleep iterations can stretch well past 200ms, especially since `-race` instrumentation both slows execution and makes goroutine scheduling less deterministic. A single overrun gap is enough to let the debounce timer fire mid-burst, producing a second rebuild once writes resume — collapsing what's meant to be one reload into two.

Confirmed unrelated to the PR that surfaced it: that diff touches only `server/services/backlog_service.go` (+test), `web-app/src/lib/store/backlogItemsSlice.ts` (+test), `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts`, and `tests/e2e/backlog-manual-override.spec.ts` — none import or exercise `session/detection`.

## Fix Approach

- Widen the safety margin: reduce the burst's real-wall-clock sensitivity by writing all 10 files back-to-back with no `time.Sleep` at all (relying on `pluginReloadDebounce` alone to collapse them), or reduce write count / increase debounce-vs-sleep ratio so the *total* burst duration budget tolerates much larger CI scheduling jitter.
- Prefer `testing/synctest` (already used elsewhere per `e592ee5ab test: convert real-wall-clock-wait tests to testing/synctest (#579)`) to fake the debounce timer's clock entirely, removing real-wall-clock dependence from this test the same way that migration did for other timing-sensitive tests.
- If neither is viable without touching `PluginWatcher`'s debounce internals, at minimum increase the `require.Never` window's tolerance or retry the burst once on the specific "second rebuild seen mid-burst" failure mode before failing, so a single scheduling-delay spike doesn't fail the whole run.

The `testing/synctest` route is the most durable fix — it matches this repo's own established pattern for exactly this problem class (see the `e592ee5ab` PR title) and eliminates the flake class entirely rather than just widening margins.

## Related Tasks

Found while shipping PR #585 (`backlog/stapler-squad-fix-list-backlog-items-allowed-transitions`). Not fixed as part of that PR — root-causing/fixing `PluginWatcher`'s debounce test timing is unrelated to, and larger in scope than, the `ListBacklogItems`/`allowedTransitions` DTO fix that PR ships.
