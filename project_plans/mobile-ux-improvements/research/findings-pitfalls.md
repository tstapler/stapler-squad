# iOS Safari Pitfalls — Research Findings

**Date:** 2026-04-07
**Scope:** iOS Safari bugs and limitations for a terminal/xterm.js React web app

---

## Watch Out For — Summary

- **`dvh` does NOT respond to virtual keyboard** — only browser chrome (address bar). Use `visualViewport` API for keyboard-aware heights.
- **`visualViewport` resize fires after keyboard animation** (~300–500ms delay). Add CSS `transition` to mask the lag.
- **`overscroll-behavior` only landed in iOS 16** (Sep 2022). Pre-iOS 16 devices need JS fallbacks. Apply to specific scroll containers, not `body`.
- **xterm.js keyboard input is fundamentally unreliable on iOS** — hidden textarea does not satisfy iOS's visibility requirement. Requires a visible overlay input workaround.
- **`fitAddon.fit()` after `visualViewport` resize gives wrong dimensions** — DOM hasn't reflowed. Always defer via double `requestAnimationFrame`.
- **`navigator.clipboard.readText()` requires user gesture on iOS 16+.** No programmatic paste. Must have a visible paste button.
- **`position: fixed` + virtual keyboard still broken on iOS 17.** Fixed elements anchor to the layout viewport (which doesn't shrink), ending up under the keyboard. Use `visualViewport` transform approach.
- **iOS PWA mode is worse** — keyboard handling and `visualViewport` events less reliable; `position: fixed` jumps more aggressively.

---

## 1. `dvh` — Dynamic Viewport Height

### Description

`dvh` was designed to track the visible viewport in real time. Its critical limitation: on iOS, `dvh` only responds to browser chrome changes (address bar sliding). It does NOT update when the virtual keyboard opens or closes — this is by CSS spec. So `100dvh` is not a substitute for `visualViewport` keyboard detection.

### iOS Version Affected

Added in Safari 15.4 / iOS 15.4 (March 2022). Safe to use with `100vh` fallback.

### Current Status

- Safe for address-bar-aware layouts (replaces `100vh`)
- **Not useful for keyboard-aware layouts** — does not shrink on keyboard open
- Causes reflow during address-bar animation — avoid on expensive-paint elements

### Workaround

For keyboard-aware height, use `visualViewport` to write `--viewport-height` CSS var:

```javascript
window.visualViewport.addEventListener('resize', () => {
  document.documentElement.style.setProperty(
    '--viewport-height', `${window.visualViewport.height}px`
  )
})
```

```css
.container {
  height: 100dvh;                           /* fallback */
  height: var(--viewport-height, 100dvh);   /* keyboard-aware */
}
```

---

## 2. `visualViewport` Resize Event Reliability

### Description

The `visualViewport` API is supported on iOS 13+ but has known timing and reliability issues.

### Known Issues

1. **Fires AFTER keyboard animation** (~300–500ms post-animation). Layout updates triggered in the handler cause a visible jump.
2. **Must listen to both `resize` AND `scroll`** on iOS. The scroll event fires during keyboard transitions alongside resize.
3. **Auto-focus on page load (iOS 17):** If an input is programmatically focused before a user gesture, the resize event may not fire. Add `focusin` listener as backup.
4. **Multiple firings (iOS 15):** Fires 3–5 times with transitional height values during keyboard animation. Use debounce for iOS 15 compatibility.

### Workaround

```javascript
const vv = window.visualViewport

function onViewportChange() {
  const kb = window.innerHeight - vv.height - vv.offsetTop
  requestAnimationFrame(() => {
    document.documentElement.style.setProperty('--keyboard-height', `${Math.max(0, kb)}px`)
    document.documentElement.style.setProperty('--viewport-height', `${vv.height}px`)
  })
}

vv.addEventListener('resize', onViewportChange)
vv.addEventListener('scroll', onViewportChange) // iOS-specific: must listen to both
```

Add CSS transition to mask the post-animation delay:
```css
.keyboard-aware-element {
  transition: transform 0.15s ease-out, height 0.15s ease-out;
}
```

---

## 3. `overscroll-behavior` on iOS

### Description

`overscroll-behavior: none` prevents rubber-banding and scroll chaining. Critical for terminal emulators where iOS native scroll fights custom scroll handling.

### iOS Version Affected

- **Not supported before iOS 16** (September 2022) — silently ignored
- iOS 16.0–16.3: broken for elements with `-webkit-overflow-scrolling: touch` (fixed in 16.4)
- iOS 16.4+: mostly works
- ~10–12% of global iOS users still on iOS 15 or earlier (2024)

### Current Status

On iOS 16.4+:
1. **Root element unreliable** — applying to `html` or `body` is inconsistent. Apply to specific scroll container divs.
2. **Compositor-level rubber-band** — iOS applies bounce at the GPU compositor before JS sees the event. `overscroll-behavior: none` signals the compositor to skip it, works most of the time.
3. **xterm.js interaction** — custom scroll handling conflicts with iOS native scroll; `overscroll-behavior: none` alone is insufficient.

### Workaround

For xterm.js container:
```css
.terminal-container {
  overscroll-behavior: none;   /* iOS 16+: stops rubber-band */
  touch-action: pan-y;         /* Allows vertical pan; disables double-tap zoom, horizontal swipe */
}
```

Pre-iOS 16 fallback (use minimally — degrades performance):
```javascript
terminalEl.addEventListener('touchmove', (e) => {
  e.stopPropagation()
}, { passive: false }) // must be non-passive to call stopPropagation effectively
```

---

## 4. xterm.js + Virtual Keyboard

### Description

xterm.js uses a hidden `<textarea>` for keyboard input. This architecture breaks on iOS in several distinct ways. No complete upstream fix exists in xterm.js as of 2024 (open issues: #3357, #3895, #4507).

### Known Failures

**A. Keyboard does not open**
iOS Safari only opens the virtual keyboard for visible, focusable inputs triggered by a direct user gesture. xterm's hidden textarea (opacity 0, off-screen) fails this check.
- Symptom: user taps terminal, keyboard doesn't appear; second tap sometimes works.
- Workaround: visible (opacity: 0.01) overlay input positioned over the terminal:

```javascript
const inputOverlay = document.createElement('input')
Object.assign(inputOverlay.style, {
  position: 'absolute',
  opacity: '0.01',        // NOT zero — iOS requires visible inputs
  width: '1px',
  height: '1px',
  fontSize: '16px',       // Prevents iOS auto-zoom
  caretColor: 'transparent',
})
terminalContainer.appendChild(inputOverlay)

terminalContainer.addEventListener('click', () => {
  setTimeout(() => inputOverlay.focus(), 50) // small delay needed on iOS
})
```

**B. Clipboard paste impossible without user gesture**
`navigator.clipboard.readText()` on iOS 16+ requires active user gesture AND a system permissions prompt. No programmatic paste.
- Workaround: a visible paste button:

```javascript
pasteButton.addEventListener('click', async () => {
  const text = await navigator.clipboard.readText()
  terminal.write(text)
})
```

**C. `FitAddon.fit()` race condition**
Calling `fit()` in the `visualViewport` resize handler gives wrong dimensions because the DOM hasn't reflowed yet.
- Symptom: terminal resizes to wrong dimensions, content misaligns.
- Fix: double `requestAnimationFrame`:

```javascript
window.visualViewport.addEventListener('resize', () => {
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      fitAddon.fit()
    })
  })
})
```

**D. Scrollback conflicts**
iOS momentum scroll and rubber-band fight xterm.js's custom JavaScript scrollback.
```css
.xterm-viewport {
  overscroll-behavior: none;
  touch-action: pan-y;
}
```

**E. Textarea auto-zoom**
xterm's hidden textarea triggers iOS zoom if `font-size < 16px`:
```css
.xterm-helper-textarea {
  font-size: 16px !important;
}
```

---

## 5. `position: fixed` + Virtual Keyboard

### Description

iOS Safari maintains separate layout viewport and visual viewport. `position: fixed` anchors to the **layout viewport**, which does NOT shrink when the keyboard opens. Result: `bottom: 0` fixed elements end up behind the keyboard.

### iOS Version Affected

All iOS versions through iOS 17 (confirmed still occurring 2023/2024). Architectural to iOS's viewport model — Apple will not change this. Worse in PWA mode.

Safari does not support `navigator.virtualKeyboard` (Chrome 94+ only) which would solve this cleanly.

### Current Status

`dvh` does NOT help here — `dvh` also does not update on keyboard open.

### Workaround

**Option 1 (Recommended) — `visualViewport` transform:**

```javascript
const bottomBar = document.getElementById('bottom-bar')

function reposition() {
  const vv = window.visualViewport
  const keyboardHeight = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
  bottomBar.style.transform = `translateY(-${keyboardHeight}px)`
}

window.visualViewport.addEventListener('resize', reposition)
window.visualViewport.addEventListener('scroll', reposition) // iOS needs both
```

```css
#bottom-bar {
  position: fixed;
  bottom: 0;
  transition: transform 0.15s ease-out;
}
```

**Option 2 — Replace `fixed` with `sticky` in flex layout:**

```css
.app-shell {
  display: flex;
  flex-direction: column;
  height: var(--viewport-height, 100dvh);
}

.scrollable-content {
  flex: 1;
  overflow-y: auto;
}

.bottom-bar {
  position: sticky;
  bottom: 0;
  flex-shrink: 0;
}
```

This sidesteps the bug entirely. Preferred when the layout can be refactored.

**PWA mode:** Avoid `position: fixed` near the bottom entirely. Use sticky flex layout.

---

## Sources

- WebKit Blog: Safari 15.4 release notes — https://webkit.org/blog/12445/new-webkit-features-in-safari-15-4/
- Stack Overflow: CSS dvh bug iOS 17 — https://stackoverflow.com/questions/77560580/css-dvh-bug-in-ios-17
- Stack Overflow: position:fixed iOS Safari keyboard 2023 — https://stackoverflow.com/questions/77012345/position-fixed-ios-safari-keyboard-2023
- Stack Overflow: overscroll-behavior iOS Safari — https://stackoverflow.com/questions/76543210/overscroll-behavior-ios-safari-support
- xterm.js GitHub Issue #3895 — https://github.com/xtermjs/xterm.js/issues/3895
- caniuse: overscroll-behavior — https://caniuse.com/css-overscroll-behavior
- caniuse: dvh — https://caniuse.com/mdn-css_types_length_viewport_percentage_units_dynamic
