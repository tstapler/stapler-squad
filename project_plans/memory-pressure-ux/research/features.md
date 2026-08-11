# Features Research: memory-pressure-ux

## hibernation_sweeper.go — Current State

Full sweep() logic in `session/hibernation_sweeper.go`:

1. **Guard**: returns early if `cfg.Hibernation.Enabled == false` or `IdleTimeoutMinutes <= 0`.
2. **Instance loading**: uses `liveProvider.GetInstances()` (fast path) or `storage.LoadInstances()`.
3. **Loop**: iterates all instances, skips non-Active ones.
4. **Idle check**: calls `inst.TimeSinceLastMeaningfulOutput(inst.CreatedAt)` — if `>= idleTimeout`, hibernates.
5. **Hibernate call**: sets reason `"idle"`, calls `inst.Hibernate(ctx)`, saves state.
6. **Log**: logs `"auto-hibernating idle session"` with session title and idle duration.

**What is missing for resource-pressure triggering:**
- No call to `cfg.Hibernation.ResourcePressureThreshold` — the field is never read.
- No system memory check at any point.
- No per-session RSS measurement.
- No sorting of instances by idle time.
- No "one session per tick" limiting for pressure-based hibernation.
- The log line does NOT include `reason=resource_pressure` for AC-1.

## TimeSinceLastMeaningfulOutput

Defined in `session/review_state.go`:
```go
func (rs *ReviewState) TimeSinceLastMeaningfulOutput(createdAt time.Time) time.Duration {
    if rs.LastMeaningfulOutput.IsZero() {
        return time.Since(createdAt)
    }
    return time.Since(rs.LastMeaningfulOutput)
}
```

`Instance` embeds `ReviewState`, so `inst.TimeSinceLastMeaningfulOutput(inst.CreatedAt)` works directly.
The method is mutex-safe only if the caller holds the lock — the sweeper calls it without a lock,
which is acceptable since `LastMeaningfulOutput` is a value type read in a tight loop.

The 5-minute meaningful-output guard (FR-2.4) can use this same method: if
`TimeSinceLastMeaningfulOutput < 5min`, skip the session for resource-pressure hibernation.

## instance_state.go — States and IsActive()

```go
func (i *Instance) IsActive() bool { return i.Status == Active }
```

States: `Creating`, `Active`, `Paused`, `Stopped`, `Hibernated`.
The sweeper correctly skips non-Active sessions. `Hibernated` sessions have no tmux
process and RSS = 0, so they'll never be candidates for resource-pressure hibernation.

## config/config.go — HibernationConfig (lines 184–200)

```go
type HibernationConfig struct {
    Enabled                   bool   `json:"enabled"`
    IdleTimeoutMinutes        int    `json:"idle_timeout_minutes"`
    ResourcePressureThreshold int    `json:"resource_pressure_threshold_pct"`  // default 85
    CheckpointDir             string `json:"checkpoint_dir"`
    RetentionDays             int    `json:"retention_days"`
}
```

`ResourcePressureThreshold` is type `int`, default 85. The sweeper only needs to read
`cfg.Hibernation.ResourcePressureThreshold` and compare against a `float64` from gopsutil.

## server/services/session_service.go — HibernateSession RPC (lines 1073–1123)

The RPC:
1. Validates `id` is non-empty.
2. Loads all instances from storage (via `s.storage.LoadInstances()`).
3. Finds the matching instance by `MatchesID(req.Msg.Id)`.
4. Sets reason via `instance.SetHibernateReason(reason)` (defaults to `"manual"`).
5. Calls `instance.Hibernate(ctx)`.
6. Saves all instances back.
7. Publishes `SessionUpdatedEvent`.
8. Returns `HibernateSessionResponse` with the updated session proto.

**Note**: The RPC uses `LoadInstances()` (disk), not the `liveProvider`. This means
the sweeper's live-provider fast path is separate from the RPC path. Memory cache
invalidation on hibernation must clear from the in-memory cache used by the sweeper.

`ResumeHibernatedSession` (lines 1128–1168) follows the same pattern with `ResumeFromHibernation(ctx)`.

## proto/session/v1/session.proto — ListSessionsResponse and Session

```proto
message ListSessionsResponse {
    repeated Session sessions = 1;
}
```

`Session` message (types.proto) uses field numbers up to 51 (VNCState). Next available
field numbers: 52+. The following optional fields are needed:
- `int32 memory_rss_mb = 52;` — per-session RSS in MB (0 if hibernated/unknown)
- `int32 estimated_savings_mb = 53;` — same as RSS for Active sessions, 0 for Hibernated

For system-wide memory, two options:
1. Add `float system_memory_pct = 54;` to `ListSessionsResponse` — requires no new RPC.
2. New `GetSystemMemory` RPC — clean separation but adds an extra round-trip.

**Recommendation**: Add `system_memory_pct` to `ListSessionsResponse` (option 1). This
matches FR-3.2 ("re-fetched whenever session list polling occurs") with zero new RPCs.

## SessionCard.tsx — Where to Inject Memory Display

`SessionCard` imports from `@/gen/session/v1/types_pb` — will automatically pick up new
`memoryRssMb` and `estimatedSavingsMb` fields once proto is regenerated.

The card has an `infoRow` section (line ~49 in CSS imports). The `badges` container
adjacent to `StatusBadge` is the right injection point for `"N MB"` display.

The `onHibernate` callback is passed at line 82 of props and forwarded to
`SessionActionsOverflow` (line 649). The tooltip/button text customization happens
inside `SessionActionsOverflow.tsx`.

## SessionRow.tsx — Where to Inject Memory Display

`SessionRow` props interface includes `onHibernate?: () => void`. The row renders
`SubStatusChip` and has an `elapsed` display. Memory would go in the `elapsed`/`actions`
area. The row is simpler than the card — a small `"N MB"` text span can be added
adjacent to the elapsed time.

## BulkActions — "Hibernate All Recommended" Pattern

`BulkActions.tsx` currently has: Pause All, Resume All, Stop All, Add Tag, Group As,
Delete All. It has `onPauseAll`, `onResumeAll`, etc. as props.

The "Hibernate all recommended" callout does NOT fit in `BulkActions` (which is for
selection-based bulk ops). Instead, it should be a **separate pressure callout component**
(e.g., `MemoryPressureCallout.tsx`) that appears above the session list. It can call
the existing `onHibernate` callback for each recommended session.

The `NotificationContext` already supports `addNotification` with `acknowledgeNotification`
and persistent dismissal patterns — this is the right system to use for the callout.
The `NotificationToast.tsx` component handles toast rendering. A custom
`MemoryPressureCallout` can either use the toast system or render inline below the header.

## 5-Second Polling Cycle

`useSessionService.ts` uses a WebSocket/SSE watch transport (`createWatchTransport`) for
real-time session events. The 5-second interval at line 658 is a **staleness detector**
(checks if no events for 15s), not a polling interval. The transport is event-driven.

Sessions are updated via `upsertSession` dispatch when `SessionEvent` arrives. Memory
fields added to `Session` proto will arrive via the same event stream when the server
pushes updates. The server must populate memory fields when publishing `SessionUpdatedEvent`
and when serving `ListSessions`.

## Edge Cases and Unstated User Needs

1. **Zero RSS display**: Hibernated sessions should show "–" not "0 MB" (FR-4.3).
2. **Threshold hysteresis**: Auto-hibernating sessions one-at-a-time (FR-2.2) prevents
   thrashing. The sweeper should re-check memory pressure after each hibernation and
   stop if pressure drops below threshold.
3. **User-visible reason**: The hibernate button label "Hibernate · saves ~N MB" is
   only visible when active — already in scope via FR-4.2.
4. **Session re-recommendation**: FR-6.4 states dismissed sessions should not re-appear
   within the browser session. This requires client-side `sessionStorage` or a `Set<string>`
   of dismissed session IDs in component state.
5. **Accuracy vs. freshness**: A 30-second TTL on RSS cache (FR-1.4) means the displayed
   value may be up to 30s stale. This is acceptable for a UX indicator.
6. **No new config keys** (NFR-3): The threshold from `ResourcePressureThreshold` covers
   all comparisons; no additional fields needed.
