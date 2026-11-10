import type { CDPSession, Page } from '@playwright/test';

/**
 * CDP-driven IME helpers for Chromium (spike 0.1.2, spike-0.1.1-0.1.2.md).
 *
 * REAL events (Chromium emits them from the CDP calls): compositionstart/update/end and
 * beforeinput/input with inputType `insertCompositionText` (isComposing=true) or `insertText`.
 * NOT real Gboard: CDP emits no `keydown` keyCode 229 around composition, produces no
 * `insertReplacementText`, and models no Android-specific Backspace shape. A pass here is
 * evidence for the layer's reducer wiring, never for keyboard behavior; the owner device
 * check (Task 0.1.3b) stays the arbiter.
 */

const sessions = new WeakMap<Page, Promise<CDPSession>>();

function cdp(page: Page): Promise<CDPSession> {
  let s = sessions.get(page);
  if (!s) {
    s = page.context().newCDPSession(page);
    sessions.set(page, s);
  }
  return s;
}

/** Update the in-flight composition to `text` (compositionstart on the first call). */
export async function imeCompose(page: Page, text: string): Promise<void> {
  const s = await cdp(page);
  await s.send('Input.imeSetComposition', { text, selectionStart: text.length, selectionEnd: text.length });
}

/** Commit `text`: compositionend if a composition is open, else a plain `insertText`. */
export async function imeCommit(page: Page, text: string): Promise<void> {
  const s = await cdp(page);
  await s.send('Input.insertText', { text });
}

/** Replace the `[start, end)` range of the focused field while composing (autocorrect-style rewrite). */
export async function imeReplaceRange(page: Page, text: string, start: number, end: number): Promise<void> {
  const s = await cdp(page);
  await s.send('Input.imeSetComposition', {
    text,
    selectionStart: text.length,
    selectionEnd: text.length,
    replacementStart: start,
    replacementEnd: end,
  });
}

/**
 * Backspace in one of the two shapes CDP can produce:
 * - `'keycode-8'`: keydown 8, `beforeinput deleteContentBackward` (even on an empty field), keyup.
 * - `'keycode-229'`: keydown/keyup with keyCode 229 and NO beforeinput, no deletion. The closest CDP
 *   gets to a keyboard that reports only a key event; it is a model, not a recording.
 */
export async function imeBackspace(page: Page, shape: 'keycode-8' | 'keycode-229' = 'keycode-8'): Promise<void> {
  const s = await cdp(page);
  const code = shape === 'keycode-8' ? 8 : 229;
  const base = { key: 'Backspace', code: 'Backspace', windowsVirtualKeyCode: code, nativeVirtualKeyCode: code };
  await s.send('Input.dispatchKeyEvent', { type: 'rawKeyDown', ...base });
  await s.send('Input.dispatchKeyEvent', { type: 'keyUp', ...base });
}

/**
 * Synthetic `beforeinput insertReplacementText` (CDP cannot produce it). The event is NOT real: it is
 * dispatched with `dispatchEvent` on the focused element, is untrusted, and does not edit the field.
 * Specs using it must say so in their header.
 */
export async function syntheticReplacementText(page: Page, data: string): Promise<void> {
  await page.evaluate((d) => {
    const el = document.activeElement;
    el?.dispatchEvent(new InputEvent('beforeinput', { inputType: 'insertReplacementText', data: d, cancelable: true, bubbles: true }));
  }, data);
}
