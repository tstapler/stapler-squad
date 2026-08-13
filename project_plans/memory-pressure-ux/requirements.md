# Requirements: memory-pressure-ux

## Problem Statement

Users are running many concurrent Claude Code sessions. The application is consuming
excessive RAM because the hibernation feature — which is supposed to automatically
free memory from idle sessions — is not functioning as intended. Specifically:

1. `ResourcePressureThreshold` exists in config (default: 85%) but the
   `HibernationSweeper` **never reads it** — it only hibernates on idle timeout, not
   memory pressure.
2. There is no per-session memory measurement, so neither the system nor the user has
   any signal about which sessions are consuming the most memory.
3. The UI exposes hibernate/resume buttons but gives users no information to guide their
   decisions (which sessions to pause, how much RAM they'd free).

## Goals

1. **Fix resource-pressure hibernation**: the sweeper must actually read system and
   per-process memory, compare against `ResourcePressureThreshold`, and auto-hibernate
   appropriate sessions.
2. **Surface per-session memory usage**: each session card/row must show its approximate
   RAM footprint.
3. **Show estimated savings**: when a session can be hibernated, show how much RAM
   pausing it would free.
4. **Global memory pressure indicator**: show a header/status-bar badge when the system
   is above the pressure threshold.
5. **Recommend sessions to pause**: when under pressure, proactively highlight idle or
   expensive sessions as hibernate candidates.
6. **Inline pressure prompt**: surface a toast or callout recommending specific sessions
   to hibernate when the system is under pressure.

## Non-Goals

- Re-architecting the state machine, checkpoint format, or tmux kill mechanism
  (these are already correct).
- SIGSTOP-based suspension (the existing kill-then-restore pattern is intentional).
- Changing the manual pause (non-hibernate) flow.

## Current Architecture (relevant parts)

### Backend

| Component | File | Notes |
|---|---|---|
| `HibernationSweeper` | `session/hibernation_sweeper.go` | Runs every 5 min, checks idle time only |
| `HibernationConfig` | `config/config.go:184` | Has `ResourcePressureThreshold` (default 85) but unused by sweeper |
| `Instance.Hibernate()` | `session/instance_hibernate.go` | Transitions state, kills tmux, writes checkpoint |
| `HibernateSession` RPC | `server/services/session_service.go:1073` | Manual trigger, `+api: session:hibernate` |
| No memory measurement | — | No `/proc/meminfo` or per-PID RSS code exists |

### Frontend

| Component | File | Notes |
|---|---|---|
| `SessionCard` | `web-app/src/components/sessions/SessionCard.tsx` | Has `onHibernate` callback, no memory info |
| `SessionRow` | `web-app/src/components/sessions/SessionRow.tsx` | Same |
| `SessionList` | `web-app/src/components/sessions/SessionList.tsx` | Passes through hibernate callbacks |
| No memory display | — | Nothing shows RAM in any session view |

## Functional Requirements

### FR-1: Memory Measurement (Backend)

**FR-1.1** The server must read system memory pressure from `/proc/meminfo` (Linux) or
`vm_stat` (macOS) at each sweeper tick and expose it as a percentage
(`usedMemoryPct = (MemTotal - MemAvailable) / MemTotal * 100`).

**FR-1.2** For each active session, the server must estimate per-session RSS by reading
`/proc/<pid>/status` (Linux) or `ps -o rss= -p <pid>` (macOS) for all processes in
the tmux session's pane tree.

**FR-1.3** PID resolution must use `tmux list-panes -t <session> -F '#{pane_pid}'` to
enumerate pane PIDs without requiring the instance to be a managed process.

**FR-1.4** Memory readings must be cached per-instance with a 30-second TTL to avoid
excessive `/proc` reads on large session counts.

### FR-2: Resource-Pressure Hibernation (Backend)

**FR-2.1** The sweeper's `sweep()` must check system memory pressure at each tick
(every 5 minutes).

**FR-2.2** When `usedMemoryPct >= ResourcePressureThreshold`, the sweeper must
hibernate **the single longest-idle Active session** per tick (one at a time, not all
at once) until pressure drops below threshold or no idle sessions remain.

**FR-2.3** Sessions hibernated for resource pressure must have their checkpoint reason
set to `"resource_pressure"` (this field already exists in `Checkpoint`).

**FR-2.4** No session that has seen meaningful output in the last 5 minutes must be
auto-hibernated for resource pressure.

### FR-3: Memory API (Backend → Frontend)

**FR-3.1** The `ListSessions` response (or a new `GetSystemMemory` RPC) must include:
- `system_memory_pct` (float, 0–100): current system-wide used-memory percentage
- Per-session: `memory_rss_mb` (int): RSS in MB (0 if unknown/hibernated)
- Per-session: `estimated_savings_mb` (int): RSS that would be freed by hibernation
  (same as `memory_rss_mb` for active sessions; 0 for hibernated)

**FR-3.2** Memory fields must be re-fetched whenever session list polling occurs
(existing 5-second poll cycle is sufficient — no new polling required).

### FR-4: Session Card / Row UI

**FR-4.1** Each session card and row must display `memory_rss_mb` when > 0, formatted
as `"N MB"` adjacent to the status badge.

**FR-4.2** When `estimated_savings_mb > 0`, the hibernate action tooltip/button must
read: `"Hibernate · saves ~N MB"`.

**FR-4.3** Hibernated sessions must show `"–"` or `"0 MB"` (not a stale figure).

### FR-5: Global Memory Pressure Indicator

**FR-5.1** A memory badge must appear in the application header when
`system_memory_pct >= ResourcePressureThreshold`.

**FR-5.2** The badge must display `"Memory: N%"` and use the `statusWarning` color token.

**FR-5.3** The badge must not appear when memory is below threshold.

### FR-6: Proactive Pause Recommendations

**FR-6.1** When `system_memory_pct >= ResourcePressureThreshold`, a non-blocking
callout (toast or inline banner) must appear listing up to 3 recommended sessions to
hibernate, sorted by `estimated_savings_mb` descending.

**FR-6.2** Each recommendation must include the session title and savings estimate.

**FR-6.3** The callout must include a "Hibernate all recommended" bulk action button.

**FR-6.4** The callout must be dismissible per-session and must not re-appear for a
dismissed session within the same browser session.

**FR-6.5** In the session list, active sessions with `estimated_savings_mb` above the
top-3 savings threshold must display a visual highlight (e.g., amber left-border) when
the system is under pressure.

## Non-Functional Requirements

- **NFR-1 Performance**: `/proc` reads must not block the sweeper for more than 100 ms
  total per tick. Cache TTL (FR-1.4) enforces this.
- **NFR-2 Cross-platform**: Memory measurement must compile and fail gracefully
  (returning 0) on macOS. Core hibernation correctness is not gated on measurement.
- **NFR-3 No new config keys**: All thresholds reuse existing `HibernationConfig` fields.
- **NFR-4 No breaking proto changes**: new fields are optional; existing callers see zero
  values.

## Acceptance Criteria

1. With `ResourcePressureThreshold = 85` and system RAM above 85%, the sweeper
   hibernates the longest-idle session within one tick (5 minutes), verified by log line
   `"auto-hibernating idle session"` with `reason=resource_pressure`.
2. After hibernation, `memory_rss_mb` for that session drops to 0 in the next poll.
3. A session card shows `"42 MB"` (or similar real value) adjacent to its status.
4. The hibernate button tooltip reads `"Hibernate · saves ~42 MB"`.
5. The global header shows `"Memory: 87%"` badge when above threshold.
6. The pressure callout lists the top-3 idle sessions with savings estimates and a
   "Hibernate all recommended" button.
7. All existing hibernation tests pass (`make test`).
8. `make quick-check` passes (build + test + lint).
