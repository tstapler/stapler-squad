# Requirements: macOS TCC Dialog Reduction

## Problem Statement

On macOS, stapler-squad generates repeated, alarming TCC (Transparency, Consent, and Control) permission dialogs that confuse users. The screenshot shows:

> "stapler-squad" would like to access Apple Music, your music and video activity, and your media library.

This dialog appears because `FocusWindow` uses AppleScript (`osascript "tell application id X to activate"`) to bring apps to front. macOS TCC requires per-target-app consent for Apple Events automation — so **each unique app the user has ever focused via stapler-squad generates its own TCC prompt**. Apple Music gets a particularly alarming dialog that mentions media library and music activity access, which looks like a privacy violation.

## Root Cause

`server/services/utility_service.go:358` runs:
```
osascript -e 'tell application id "com.apple.Music" to activate'
```
macOS requires `kTCCServiceAppleEvents` consent for every distinct target bundle ID. When the target is `com.apple.Music`, the dialog text says "access Apple Music, your music and video activity, and your media library" — alarming and confusing for a developer tool.

Contributing factors:
1. **Per-app TCC proliferation** — every unique target bundle ID gets its own TCC prompt
2. **Codesign not set up** — without `make setup-codesign`, rebuilds invalidate TCC grants (binary cdhash changes), causing prompts to repeat after every `make install-service`
3. **No user guidance** — the app provides no in-UI explanation when a TCC prompt is expected
4. **Music as source app** — `source_bundle` metadata in notification events can resolve to `com.apple.Music` if Music is frontmost when a session is created

## Goals

1. Eliminate or drastically reduce unsolicited TCC prompts for Apple Music and other apps
2. Replace per-app Apple Events automation with a mechanism that doesn't require per-app TCC consent
3. Provide clear user guidance when a one-time TCC grant is genuinely needed
4. Make the first-run experience smooth for new users who haven't run `make setup-codesign`

## Out of Scope

- Changing the FocusWindow _feature_ itself (still bring windows to front)
- Supporting Linux/Windows window focus improvements (separate effort)
- Replacing the codesign mechanism entirely

## Acceptance Criteria

1. FocusWindow on macOS no longer triggers Apple Events TCC dialogs for individual target apps
2. The Apple Music TCC dialog ("would like to access Apple Music, your music and video activity") is never shown
3. Users who have not run `make setup-codesign` do not see repeated TCC prompts after rebuilds
4. When a one-time macOS permission is genuinely needed, the UI surfaces a clear explanation before the OS dialog appears
5. `source_bundle` values that are non-developer apps (e.g., `com.apple.Music`, `com.apple.TV`) are either sanitized server-side or result in a no-op FocusWindow call with a meaningful log message
6. Unit tests cover the new FocusWindow implementation on darwin
7. Integration test confirms `make setup-codesign` sets up stable TCC grants that survive a rebuild
