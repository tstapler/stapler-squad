# Requirements: Rate Limit Detection & Auto-Resume

**Project**: detect-rate-limit  
**Date**: 2026-05-02  
**Status**: Draft

## Context

Stapler Squad manages Claude Code sessions. When Claude hits its usage limit, it shows a message like:

> "You've hit your limit - resets 11pm (America/Los_Angeles)  
> /extra-usage to finish what you're working on."

Sessions stall until the user manually returns and types "continue". The backend already has a partial rate limit detection system (`session/detection/ratelimit/`) that detects patterns, schedules recovery, and sends PTY input — but critical pieces are missing:

1. The specific Claude rate-limit message format is not matched by existing patterns
2. Rate limit state never reaches the frontend (adapter gap in `instance_adapter.go`)
3. No notifications are triggered on detection or recovery
4. Re-detection after failed recovery attempts is not implemented
5. Per-session enable/disable is not exposed to the UI

## User Stories

### US-1: Detect Claude Rate Limit
**As a** developer running Claude sessions  
**I want** Stapler Squad to detect when Claude hits its usage limit  
**So that** I know the session is paused and when it will resume

**Acceptance Criteria**:
- Pattern matches: "You've hit your limit - resets Xpm (Timezone)"
- Pattern matches: "You've hit your limit - resets at HH:MM AM/PM"
- Pattern matches existing patterns (Usage limit reached, /rate-limit-options, etc.)
- Timezone-aware timestamp parsing for formats: "11pm America/Los_Angeles", "11:30pm Pacific", "11pm (America/Los_Angeles)"
- Detection occurs within 2 seconds of the pattern appearing in PTY output
- Duplicate detection suppressed during cooldown period (30s default)

### US-2: Auto-Resume at Reset Time
**As a** developer with a rate-limited session  
**I want** Stapler Squad to automatically send "continue" when the limit expires  
**So that** my session resumes without manual intervention

**Acceptance Criteria**:
- Parse reset time from detected message and schedule recovery at that exact moment (+ 5s buffer)
- Send "1\n" (Anthropic) or appropriate provider input at scheduled time
- If reset time cannot be parsed: fall back to 30-minute retry
- After sending recovery input, continue monitoring output for re-detection
- If rate limit detected again after recovery: parse new reset time and reschedule (up to implicit N retries; re-detect and reschedule indefinitely)
- Do not send recovery input if session is stopped/paused

### US-3: Notifications
**As a** developer away from the desk  
**I want** to be notified when my session hits a rate limit and again when it resumes  
**So that** I can track progress without watching the terminal

**Acceptance Criteria**:
- Desktop push notification on detection: "Session X rate limited — resumes at 11pm"
- Desktop push notification on recovery success: "Session X resumed after rate limit"
- Desktop push notification on recovery failure: "Session X failed to resume after rate limit"
- In-app session card badge: clock icon + "Rate limited until 11pm" shown while in waiting state
- In-app session card badge cleared when state returns to none/recovered
- Web UI toast on detection: non-blocking, shows reset time
- Web UI toast on recovery: confirms session resumed
- Notifications respect existing NotificationRateLimiter (no spam)

### US-4: Per-Session Configuration
**As a** developer  
**I want** to control auto-resume per session  
**So that** I can opt out for sessions where manual review is preferred

**Acceptance Criteria**:
- Global setting: auto-resume enabled by default
- Per-session toggle: shown in session card or session detail view
- Toggle state persisted through session restart
- Toggling off cancels any pending scheduled recovery
- Toggling on re-enables detection for future rate limits

### US-5: Rate Limit State in Frontend
**As a** frontend  
**I want** rate limit state surfaced in the session proto  
**So that** the UI can render accurate status

**Acceptance Criteria**:
- `RateLimitState` proto field populated in `InstanceToProto()` adapter
- `ResetTime` (timestamp) included in proto when state is Waiting
- State transitions (None → Waiting → Recovering → Recovered/Failed) visible via WebSocket stream
- `SessionCard.tsx` renders rate-limit-specific UI correctly (no undefined references)

## Out of Scope
- Custom notification channels (Slack, email) — existing push notification system sufficient
- Changing the recovery input (user-configurable) — default per-provider inputs adequate
- Multi-provider UI differentiation — provider info surfaced in logs only
- Historical rate limit analytics / charts

## Technical Constraints
- Extend existing `session/detection/ratelimit/` package — do not replace
- Use existing `NotificationService` for notifications — do not introduce new channels
- Proto changes require `make generate-proto`
- Frontend changes must use vanilla-extract for new CSS; existing module.css tokens only for edits
- Re-detection loop must be bounded (use existing cooldown mechanism to prevent infinite tight loops)
- All new Go code must pass `make lint`

## Definition of Done
- `make build && make test` passes
- Rate limit state visible in session card UI for a session that triggers the pattern
- Desktop notification fires on detection and recovery
- Per-session toggle works end-to-end
- No regressions in existing detection tests
