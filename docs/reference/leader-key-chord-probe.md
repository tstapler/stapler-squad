# Leader-key chord probe

The command bar opens on a configurable leader chord (default **Alt+;**, matched on
`KeyboardEvent.code === "Semicolon"` with `altKey`). Code: `web-app/src/lib/shortcuts/leaderConfig.ts`.

## Why `event.code`

On macOS, Option+letter produces a symbol (`event.key === "∑"` for Option+W), so a
`key`-based match never fires there. `code` is the physical key and is layout-independent.

## Methodology

For each browser/OS cell, focus the page body, then run in the DevTools console:

```js
document.addEventListener('keydown', e => console.log(
  { key: e.key, code: e.code, alt: e.altKey, ctrl: e.ctrlKey, shift: e.shiftKey, meta: e.metaKey,
    prevented: e.defaultPrevented, composing: e.isComposing }), true);
```

Press the candidate chord and record: (1) the listener fires at all (the OS or browser
chrome did not consume it), (2) `code` is as expected, (3) no visible browser or OS
side effect (menu, address bar, character inserted into a focused terminal).

## Results

| Cell | Alt+; result | Source |
|---|---|---|
| Linux, Chromium (headless, Playwright/CDP key events) | listener fires; `key=";"`, `code="Semicolon"`, `altKey=true` | measured in this change (`tests/e2e/leader-command-bar.spec.ts` arms the bar with it) |
| Linux, Chrome (headed, real keyboard) | **not yet verified** | needs a human |
| macOS, Chrome | **not yet verified** | needs a human |
| macOS, Safari | **not yet verified** | needs a human |

The Linux row is a synthetic-event measurement: it proves the matcher and that the
page-level chord handling works, not that a real desktop environment delivers the
keystroke. The macOS cells and headed Linux were not available when this was written
(sandboxed environment, no macOS host), so **the default is provisional** until the
three human cells above are filled in. Do not treat this document as a completed probe.

## Why Alt+; as the provisional default

- Alt (not Ctrl): Ctrl+letter is a PTY control byte (Ctrl+C, and Ctrl+A is the common
  tmux/screen prefix), and Ctrl+Space is NUL. The settings UI rejects chords without
  Alt or Meta for this reason.
- `;` has no Alt binding in Chrome/Firefox/Safari menus (unlike Alt+F/E/D), and no
  default readline/emacs Meta binding that matters to terminal users.
- The existing window leader Alt+W is untouched.

## "No safe chord" fallback

If a cell shows the chord is consumed or collides, users can:

1. Settings > Keyboard Shortcuts > **Change leader key** and record another Alt/Meta chord
   (stored in `localStorage` key `ssq.leaderKey`), or
2. choose **No safe chord: disable leader**, which removes the global chord entirely;
   the bar still opens from **Open command bar** on the same page.

Alt+W (opens the bar at the `w` branch) remains available either way.
