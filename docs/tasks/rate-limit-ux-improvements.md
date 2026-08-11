# Rate Limit UX Improvements

## Overview

This document captures UX improvements identified during review of the rate limit detection feature. Issues are categorized by priority with actionable tasks.

---

## HIGH Priority Tasks

### Task 1: Add Per-Session and Global Disable for Rate Limit Feature

**Status**: ⏳ Pending

**Description**: Users cannot disable the rate limit detection feature either globally or per-session. Add configuration options to enable/disable the feature.

**Acceptance Criteria**:
- Global config option to enable/disable rate limit detection
- Per-session override option (UI toggle in session settings)
- When disabled, detection does not run but detection state shows "disabled" in UI

**Files Likely Affected**:
- `session/detection/ratelimit/manager.go` - Add enabled state
- `config/config.go` - Add global config option
- `server/services/session_service.go` - Add per-session option
- Web UI components - Add toggle control

**Priority**: HIGH

---

### Task 2: Add Status Display in Web UI

**Status**: ⏳ Pending

**Description**: No status is displayed in the web UI when rate limits are detected. Add visual indicators showing current rate limit state per session.

**Acceptance Criteria**:
- Session card shows rate limit status indicator (icon/badge)
- Detail view shows: detected provider, reset time, recovery status
- Status colors: yellow (waiting), green (recovered), red (failed)
- Filter by rate limit status in session list

**Files Likely Affected**:
- `web-app/src/components/SessionCard.tsx` - Add status indicator
- `proto/session/v1/session.proto` - Add rate limit status to session model
- `server/services/session_service.go` - Expose rate limit state

**Priority**: HIGH

---

### Task 3: Add User Notification on Recovery Failure

**Status**: ⏳ Pending

**Description**: When recovery fails, there is no user notification. Implement user-facing alerts when automatic recovery fails.

**Acceptance Criteria**:
- Toast/notification appears when recovery fails
- Notification includes: session name, provider, error reason
- "Retry manually" action button in notification
- Recovery failure logged with details for debugging

**Files Likely Affected**:
- `session/detection/ratelimit/recovery.go` - Add failure handling
- `server/services/notification_service.go` - Add notification
- Web UI - Add toast notification component

**Priority**: HIGH

---

### Task 4: Add Logging in Detector When Patterns Match

**Status**: ⏳ Pending

**Description**: The detector does not log when rate limit patterns are matched, making debugging difficult. Add comprehensive logging at detection points.

**Acceptance Criteria**:
- Log at INFO level when rate limit pattern detected
- Log includes: session ID, matched pattern, provider detected
- Log at DEBUG level for detailed matching info
- Structured logging with appropriate fields

**Files Likely Affected**:
- `session/detection/ratelimit/detector.go` - Add detection logging

**Priority**: HIGH

---

## MEDIUM Priority Tasks

### Task 5: Make Buffer Time Configurable

**Status**: ⏳ Pending

**Description**: The 30-second buffer time before recovery is hardcoded. Make it configurable.

**Acceptance Criteria**:
- Config option for buffer time (default 30 seconds)
- Range validation (5-120 seconds)
- Per-session override capability
- Shows configured value in UI

**Files Likely Affected**:
- `session/detection/ratelimit/scheduler.go` - Use config value
- `config/config.go` - Add config option
- `docs/tasks/detect-and-address-rate-limits.md` - Update config struct

**Priority**: MEDIUM

---

### Task 6: Make Patterns Configurable

**Status**: ⏳ Pending

**Description**: Rate limit detection patterns are hardcoded. Allow users to add custom patterns.

**Acceptance Criteria**:
- Config file support for custom patterns
- Pattern format: regex + provider name + recovery input
- UI to view/edit custom patterns
- Validation of regex patterns on save

**Files Likely Affected**:
- `session/detection/ratelimit/detector.go` - Load patterns from config
- `config/config.go` - Add patterns array
- Web UI - Add pattern management view

**Priority**: MEDIUM

---

### Task 7: Add Recovery History for Debugging

**Status**: ⏳ Pending

**Description**: No history of past rate limit recoveries is stored, making debugging difficult. Add recovery history tracking.

**Acceptance Criteria**:
- Store last N recovery attempts (configurable, default 10)
- History includes: timestamp, provider, success/failure, reset time
- View history in session detail page
- API endpoint to fetch history

**Files Likely Affected**:
- `session/detection/ratelimit/manager.go` - Add history storage
- `session/storage.go` - Persist history
- `proto/session/v1/session.proto` - Add history to model
- Web UI - Add history display component

**Priority**: MEDIUM

---

## LOW Priority Tasks

### Task 8: Add Help Text About Feature

**Status**: ⏳ Pending

**Description**: No help text explains what the rate limit detection feature does. Add explanatory text.

**Acceptance Criteria**:
- Help icon/tooltip on session card with rate limit status
- Documentation link in settings area
- First-time user explanation modal (optional)

**Files Likely Affected**:
- Web UI components - Add help text/tooltip

**Priority**: LOW

---

### Task 9: Add Cooldown Configuration

**Status**: ⏳ Pending

**Description**: No configurable cooldown between recovery attempts. Add cooldown option.

**Acceptance Criteria**:
- Config option for cooldown period (default 60 seconds)
- Prevents rapid retry attempts after failure
- Shows cooldown status in UI

**Files Likely Affected**:
- `session/detection/ratelimit/manager.go` - Add cooldown logic
- `config/config.go` - Add config option

**Priority**: LOW

---

### Task 10: Add Recovery Feedback in Terminal

**Status**: ⏳ Pending

**Description**: No feedback in the terminal when recovery is triggered. Add visual/audio feedback.

**Acceptance Criteria**:
- Send status message to terminal on recovery start
- Send confirmation message on success
- Optional: system notification on recovery complete
- Configurable feedback options

**Files Likely Affected**:
- `session/detection/ratelimit/recovery.go` - Add terminal output

**Priority**: LOW

---

## Summary

| Priority | Count | Estimated Effort |
|----------|-------|------------------|
| HIGH     | 4     | 8-12 hours       |
| MEDIUM   | 3     | 6-9 hours        |
| LOW      | 3     | 3-4 hours        |

**Total**: 10 tasks, ~17-25 hours estimated

---

## Dependencies

- Task 1 (Disable) enables Task 2 (UI Status) - need state to display
- Task 1 (Disable) should be completed early for UX flexibility
- Task 4 (Logging) provides debugging foundation for other tasks

---

## Notes

- CRITICAL issues (#1-3) from UX review were partially addressed in initial implementation but gaps remain (user visibility, logging, hardcoded input)
- Hardcoded "1\n" recovery input should be addressed in Task 3 (Recovery Failure) as part of making recovery inputs configurable
- Status visibility in UI (#5) is HIGH priority - users need to know what's happening