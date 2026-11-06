# UX Research: macOS TCC Dialog Reduction

Feature: Eliminating alarming TCC permission dialogs when users click Focus Window, plus first-run codesign setup guidance.

---

## 1. Job-to-be-done: what the user wants when clicking Focus Window

When a notification toast appears and the user clicks Focus Window, their job is:

> "Return my attention to the tool that needs it — let me instantly see the terminal or editor where my AI agent is waiting, without having to cmd-tab through open windows."

This is a high-urgency, low-friction action. The user is context-switching because an agent needs approval or has finished a task. Any friction — especially a confusing macOS dialog about "Apple Music, your music and video activity, and your media library" — breaks trust and derails the flow.

The confusing dialog is the core UX problem. The user did not ask to touch their media library. The dialog appears because, without proper code signing with a bundle ID, macOS categorises the Apple Events TCC grant under a more generic or incorrect TCC category. With proper codesigning (`com.stapler-squad` bundle ID + `NSAppleEventsUsageDescription` in `Info.plist`), macOS would show the correct purpose string: "stapler-squad uses Apple Events to bring your editor or browser to the foreground when you click Focus Window." The setup is a prerequisite for the button to work cleanly.

---

## 2. How other macOS developer tools handle first-run permissions

Three patterns dominate among developer tools that need non-trivial macOS permissions:

### Pattern A: Contextual request with pre-dialog explanation (preferred)

iTerm2, BetterTouchTool, and Raycast all show an in-app dialog or tooltip BEFORE triggering the macOS TCC dialog. The in-app explanation says: "To do X, macOS will ask you for permission. Click OK in the next dialog." This mental preparation makes the system dialog feel expected rather than alarming.

The key UX insight: the TCC dialog alone is not scary. What makes it scary is when it appears without context — especially when it names a resource (like Apple Music) that the user never expected the tool to touch.

### Pattern B: Capabilities gated until permission is granted

VS Code extensions that need permissions (like the macOS Accessibility bridge for screen reader support) show a persistent notification badge on the extension icon with text like "Requires accessibility permission — click to set up." The capability is disabled and labeled as such until the one-time grant is made. This avoids surprising the user mid-action.

### Pattern C: Settings page with permission status display

Tools like Docker Desktop and 1Password show a dedicated "Permissions" or "macOS Integration" section in their settings page. Each permission has:
- A status indicator (granted / not granted / unknown)
- A one-click button to open the relevant System Settings pane
- A plain-English explanation of why the permission is needed

This pattern is effective when permissions are one-time setup requirements rather than per-use requests.

**Recommendation for stapler-squad**: use Pattern A (pre-dialog explanation) the first time Focus Window is clicked without a confirmed grant, and Pattern C (settings page status) for ongoing visibility.

---

## 3. In-UI patterns for "this requires a one-time macOS permission"

### Pattern: Status-aware button with inline setup guidance

The strongest pattern (used by Raycast, CleanMyMac) is a button that shows different states depending on permission status:

- **Not set up**: Dimmed label + lock or warning icon, tooltip explaining what to run
- **First click / setup needed**: Instead of attempting the action and failing, show an inline card: "Focus Window requires a one-time setup. Run `make setup-codesign` to enable it."
- **Pending system dialog**: Pre-dialog card: "Click OK when macOS asks about Apple Events — this lets stapler-squad switch windows for you."
- **Granted / working**: Button works silently; toast closes after a short delay

### What makes the TCC dialog alarming

macOS's TCC dialog wording is controlled by the `NSAppleEventsUsageDescription` key in the embedded `Info.plist`. The current `macos/Info.plist` has the correct string. However, if the binary is NOT code-signed with the `com.stapler-squad` bundle ID (i.e., the user never ran `make setup-codesign`), macOS falls back to a generic or incorrect TCC category, producing the misleading "Apple Music / media library" dialog.

Without codesigning, two things go wrong. First, if the user has run `make setup-codesign`, the dialog (when it appears) shows the correct, non-alarming reason string from `NSAppleEventsUsageDescription`. Second, if the user has NOT run `make setup-codesign` — the common case for new installs — macOS falls back to a generic or mismatched TCC category, producing the alarming media library dialog.

The primary fix is ensuring the binary is always codesigned before Focus Window is offered in the UI. The secondary fix is surfacing a clear setup path when it hasn't been done.

### Existing permission-status pattern in codebase

`PushNotificationSettings.tsx` shows a good precedent: when `permission === "denied"`, it renders a "Blocked" status badge plus plain-English instructions. The same pattern applies to Focus Window's unset-up state:

```
[⚠ Setup needed]
Focus Window requires a one-time code signing setup.
Run: make setup-codesign    [Copy command]
Then restart the service.   [Learn more →]
```

---

## 4. Current Focus Window button — exact UI state

File: `web-app/src/components/ui/NotificationToast.tsx`, lines 213–216 and 419–424.

### Visibility logic

```tsx
const focusWindowAvailable = Boolean(hasSourceApp && notification.onFocusWindow) && !sessionHidden;
// Inline: desktop only, not approval toasts
const showFocusInline = focusWindowAvailable && !coarse && !(stacked && isApproval);
// Overflow "..." menu: desktop approval toasts
const focusInOverflow = stacked && !coarse && focusWindowAvailable && isApproval;
```

### Rendered button

```tsx
<button
  className={focusButton}
  onClick={notification.onFocusWindow}
  title="Focus the source application window"
>
  🔗 Focus Window
</button>
```

### CSS

`focusButton` (`NotificationToast.css.ts` line 286): transparent background, primary-color border, text in primary color. On hover: solid primary fill with on-primary text. This is visually prominent — a non-destructive secondary action.

### Current failure behavior

When the backend's `FocusWindow` RPC returns `success: false` (e.g., because AppleScript failed due to TCC denial), the toast does NOT display any error message. The response's `message` field ("Permission denied. Please grant Accessibility permissions...") is silently dropped. The user clicks the button, nothing happens, and no feedback is given.

---

## 5. What the Focus Window button should show by state

| State | Label | Style | Behavior on click |
|---|---|---|---|
| `onFocusWindow` absent or session hidden | Button not rendered | — | — |
| Binary not codesigned (detectable via `/api/v1/codesign-status` endpoint or similar) | 🔒 Focus Window | Dimmed, muted border | Show inline setup card in toast |
| Codesigned, permission not yet granted (first use) | 🔗 Focus Window | Normal | Call RPC; if fails, show inline one-time permission instructions |
| Codesigned, permission granted (working) | 🔗 Focus Window | Normal | Call RPC silently; on success close toast after 800ms |
| App not running | 🔗 Focus Window | Normal | Show toast inline: "App is not running" |

The "dimmed until codesigned" state eliminates the alarming dialog entirely for users who haven't done first-time setup. It also sets expectations: the feature exists, but needs one step.

---

## 6. How to surface codesign setup guidance

Three options, ranked by friction for the user:

### Option A: Settings page section (recommended for ongoing visibility)

Add a "macOS Permissions" section to the existing Settings page (alongside PushNotificationSettings). It shows:
- Codesign status: Configured / Not configured
- Apple Events TCC grant status: Granted / Not granted / Unknown
- Setup instructions when not configured
- A "Verify" button that runs `make verify-codesign` equivalent via the server

This is always-accessible and easy to discover from a "why isn't Focus Window working?" search path.

### Option B: First-launch dismissible banner (recommended for discoverability)

When the server detects the binary is not codesigned (no `CFBundleIdentifier`, or `codesign -dv` check fails), surface a banner at the top of the main session list:

```
⚠ macOS Focus Window setup needed
Run make setup-codesign once to enable window switching and prevent unexpected permission dialogs.
[Copy command]  [Dismiss]
```

Pattern precedent: VS Code's extension API deprecation banners, iTerm2's first-launch "grant accessibility" prompt.

### Option C: Contextual inline card in the toast (recommended for the first failure)

When Focus Window is clicked and the backend returns a permission error or the binary check fails:

```
🔒 Focus Window needs a one-time setup
Run make setup-codesign to enable this feature without
unexpected macOS permission dialogs.
[Copy command]  [Dismiss]
```

This replaces silent failure with actionable guidance exactly when the user encounters the problem.

### Recommended combination

- Option A (settings section): always present for users who go looking
- Option C (inline toast card): on first failure, once per install
- Do NOT use Option B (first-launch banner) unless the setup is truly zero-friction — a banner for a two-step setup process trains users to dismiss banners before reading them

---

## 7. Summary of macOS codesign current state

From `website/src/content/docs/help/macos-codesigning.md` and `entitlements.plist`:

- The entitlement `com.apple.security.automation.apple-events` is correct
- `Info.plist` has `NSAppleEventsUsageDescription` with the right user-facing string
- The codesigning flow is: `go build` with CGO embeds the plist, then `codesign --sign StaplerSquadDev --entitlements entitlements.plist`
- `make setup-codesign` must be run once per developer machine to create the `StaplerSquadDev` self-signed cert
- Without this cert, every `make install-service` produces a binary with a new cdhash, TCC treats it as a new app, and the wrong (alarming) dialog appears

There is currently no in-app surface that tells the user whether codesigning has been done.

---

## Key tensions and tradeoffs

**Gating the button vs. always showing it**: Gating (dimmed/disabled until codesigned) prevents the bad dialog but may confuse users who don't know what codesigning is. Showing it always with graceful failure gives users the button immediately but risks the alarming dialog on first use. Gating is lower friction overall because the alarming dialog destroys trust in a way a grayed-out button does not.

**Server-side vs. client-side detection**: The codesign status check needs to run on the server (the Go binary knows its own signing state). The client (React) needs a new RPC or a field on an existing status endpoint. This is a small API change but necessary to drive the button state accurately.

**"One-time" framing**: Users respond well to "one-time setup" framing. Emphasize that once done, the button works silently forever. The current docs and error messages don't use this framing.
