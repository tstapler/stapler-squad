# Architecture Research: TerminalOutput.tsx Toolbar & New Hook Integration

## How devOnly Toolbar Buttons Are Wired

### The `devOnly` CSS class
Defined in `web-app/src/components/sessions/TerminalOutput.css.ts` (line 188-194):
```ts
export const devOnly = style({
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});
```
`devOnly` does **not** use `process.env.NODE_ENV` — it is purely a responsive CSS class that hides buttons on mobile (≤768px). It does **not** hide buttons in production builds on desktop. This is important: "dev only" here means "desktop only" in practice.

### Existing debug toggle button pattern (lines 991-999 / 1000-1015)
Two existing `devOnly` buttons in `TerminalOutput.tsx`:

**1. Debug logging toggle** (lines 991-999):
```tsx
<button
  className={`${styles.toolbarButton} ${styles.devOnly} ${debugMode ? styles.debugActive : ''}`}
  onClick={handleToggleDebug}
  title={debugMode ? "Disable debug logging" : "Enable debug logging"}
  style={debugMode ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
>
  🛠️ {debugMode ? 'Debug ON' : 'Debug'}
</button>
```

`debugMode` is state initialized from `localStorage.getItem("debug-terminal")`. The `handleToggleDebug` callback:
- Flips the state with `setDebugMode`
- Writes `localStorage.setItem("debug-terminal", "true")` or `localStorage.removeItem("debug-terminal")`
- Calls `console.log` to announce the change

**2. Recording toggle** (lines 1000-1015):
```tsx
<button
  className={`${styles.toolbarButton} ${styles.devOnly}`}
  onClick={() => {
    if (isRecording) { stopRecording(); setIsRecording(false); }
    else { startRecording(); setIsRecording(true); }
  }}
  style={isRecording ? { backgroundColor: '#ff4444', color: 'white' } : {}}
>
  {isRecording ? '⏹️ Stop Rec' : '⏺️ Record'}
</button>
```

Both buttons appear inside `{toolbarExpanded && <div className={styles.toolbarActions}>}` — the collapsible toolbar section. The new browser-log-stream button should follow the same pattern.

### Full toolbar render structure (lines 941-1132)
```
<div className={styles.toolbar}>
  <div className={styles.status}>       ← left side: connection indicator
  <div className={styles.actions}>      ← right side: all buttons
    <button toolbarToggle>              ← mobile expand/collapse
    {showReconnectButton && <button>}   ← always visible reconnect
    {toolbarExpanded && (
      <div className={styles.toolbarActions}>
        <button devOnly>  🛠️ Debug    ← EXISTING devOnly button slot
        <button devOnly>  ⏺️ Record   ← EXISTING devOnly button slot
        <select devOnly>  mode select ← EXISTING devOnly select
        ...primary buttons...
        <div mobileMoreWrapper>
          <div secondaryGroup>         ← secondary buttons (collapsed on mobile)
            ...secondary buttons...
          </div>
          <button mobileMoreButton>    ← ··· trigger
        </div>
      </div>
    )}
  </div>
</div>
```

**New button placement**: The browser-log-stream toggle should go alongside the Debug and Record buttons (the `devOnly` cluster at the top of `toolbarActions`), since it is a developer diagnostic feature.

## How to Add a New Hook Alongside Existing Ones

### Location
`web-app/src/lib/hooks/useBrowserLogStream.ts` — follows the `use<Feature>.ts` naming convention. All existing hooks in this directory are a single file with one exported function.

### Hook signature (proposed)
```ts
export interface UseBrowserLogStreamOptions {
  enabled: boolean;    // controlled by the toolbar button state
  sessionId?: string;  // optional: tag logs with current session
  endpoint?: string;   // default: '/api/v1/browser-logs'
}

export function useBrowserLogStream(options: UseBrowserLogStreamOptions): void
```

The hook returns nothing (like `usePushNotifications`'s internal side-effects) or returns a `{ flush: () => void }` escape hatch.

### Integration in TerminalOutput.tsx

1. Import the hook at the top of the file (alongside existing imports).
2. Add state for the toggle (mirrors `debugMode`):
   ```ts
   const [logStreamEnabled, setLogStreamEnabled] = useState(() => {
     if (typeof window !== 'undefined') {
       return localStorage.getItem('browser-log-stream') === 'true';
     }
     return false;
   });
   ```
3. Call the hook with the enabled flag:
   ```ts
   useBrowserLogStream({ enabled: logStreamEnabled, sessionId });
   ```
4. Add a handler (mirrors `handleToggleDebug`):
   ```ts
   const handleToggleLogStream = useCallback(() => {
     const next = !logStreamEnabled;
     setLogStreamEnabled(next);
     if (typeof window !== 'undefined') {
       if (next) localStorage.setItem('browser-log-stream', 'true');
       else localStorage.removeItem('browser-log-stream');
     }
   }, [logStreamEnabled]);
   ```
5. Add the button in the `devOnly` cluster:
   ```tsx
   <button
     className={`${styles.toolbarButton} ${styles.devOnly} ${logStreamEnabled ? styles.debugActive : ''}`}
     onClick={handleToggleLogStream}
     title={logStreamEnabled ? "Stop forwarding console logs to server" : "Forward console logs to server"}
     style={logStreamEnabled ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
   >
     📡 {logStreamEnabled ? 'Log Stream ON' : 'Log Stream'}
   </button>
   ```

## Key Implementation Details

- `debugActive` (from `TerminalOutput.css.ts` line 138) is an empty `style({})` — it's a semantic class only; the active styling is done via inline `style=` on the button. The new button should follow the same pattern.
- The `toolbarExpanded` state controls whether the `toolbarActions` div renders at all — the new button is inside this div, so it will collapse/expand with the toolbar automatically.
- On mobile, `devOnly` hides the button entirely (display:none). No extra mobile handling needed.
