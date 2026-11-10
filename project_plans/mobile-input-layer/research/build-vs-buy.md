# Build vs Buy: mobile soft-keyboard input layer

Date: 2026-11-06. Labels: VERIFIED = fetched this session (URL given); INFERRED = my reasoning.

## Recommendation

Build a thin, renderer-agnostic input layer in-repo (hidden textarea + `beforeinput`/composition state machine + a small hand-written common-prefix/suffix rewrite diff). Add **zero new runtime dependencies**. Reuse `fast-check` (already a devDependency) for property tests, and replay recorded event traces in Jest and Playwright. No credible library or upstream fix exists that makes this renderer-agnostic. Do not fork or patch either renderer.

## Repo facts (VERIFIED, local)

- `web-app/package.json`: `@xterm/xterm ^6.0.0`, `ghostty-web 0.4.0` (exact), `@codemirror/view ^6.41.0` (+ state, commands; used for file viewing/editing), `fast-check ^4.10.2`, `@testing-library/user-event ^14.5.2`, `jest-environment-jsdom ^30.2.0`, `@playwright/test ^1.57.0`.
- No `diff`, `fast-diff` or `diff-match-patch` dependency and no import of one in `web-app/src`.
- `web-app/bunfig.toml`: `minimumReleaseAge = 259200` (3 days), `linker = "isolated"`. Bun is the only package manager (`preinstall` guard rejects npm/yarn/pnpm). `trustedDependencies` is an explicit allowlist.
- `patchedDependencies` already exists, with one entry (`eval@0.1.8`, a transitive pin), so Bun's patch mechanism is in use.
- Local `node_modules` was absent in the worktree, so installed versions were not re-checked. Registry values below come from npm.
- `tests/e2e/` has `android-url-bar.spec.ts`, so mobile-emulated Playwright specs have precedent.

## 1. Existing OSS

### 1a. xterm.js upstream (`@xterm/xterm`)

- Latest on npm is 6.0.0, MIT (https://registry.npmjs.org/@xterm/xterm/latest). VERIFIED. The repo already uses `^6.0.0`.
- Release notes (https://github.com/xtermjs/xterm.js/releases): 6.0.0 has "Fix duplicate input for some IMEs" (#5024). 5.0.0 has "Improve deletion handling on Android" (#4007). Nothing in the visible text mentions `beforeinput` or composition rewrites. I read only the first 100k of 134k characters, so 4.18/4.19 notes are not covered. VERIFIED, partial.
- Issue #3600 (https://github.com/xtermjs/xterm.js/issues/3600) is still **open**, labelled `area/mobile`, `help wanted`, with no linked PR. The latest comment (2026-04-12) says Enter/Backspace duplication from composition bursts remains. It proposes a CodeMirror-`domobserver`-style textarea-delta fix. The commenter has no Android rig, so it is untested. The maintainer called it low priority in 2022. VERIFIED.
- Issue #5377 (https://www.goodfirstissue.org/xtermjs/xterm.js/issues/5377) says touch and virtual-keyboard support is basic. VERIFIED via a search snippet only.
- Verdict: **Not recommended** as the fix. It is the right baseline for the xterm renderer, and bumping within 6.x costs nothing, but a newer release will not fix Gboard.

### 1b. ghostty-web

- Latest on npm is 0.4.0, MIT, no runtime deps (https://registry.npmjs.org/ghostty-web/latest). VERIFIED. 0.4.0 is the "Latest" release, dated 09 Dec (year not shown; https://github.com/coder/ghostty-web/releases). It lists IME support (#90) and iOS support (#76) only. VERIFIED.
- PR #110 "Android support" is **merged to main on 2026-01-03** (https://github.com/coder/ghostty-web/pull/110). It adds `beforeinput` handling, with dedupe against `paste` and against `compositionend` vs `beforeinput insertText`. It does not mention keyCode 229 or rewrites. Merged after 0.4.0, and there is an open request for a new npm release. So the published 0.4.0 we run likely lacks it. VERIFIED that the PR is merged; INFERRED that 0.4.0 predates it (0.4.0 is the latest npm version even though the merge was 3+ weeks later).
- The requirements doc says 0.4.0 `preventDefault()`s every `beforeinput` and ignores keyCode 229. That is a local read of `dist/ghostty-web.js`, so main may differ.
- Open IME PRs/issues (https://github.com/coder/ghostty-web/pulls?q=is%3Apr+IME+OR+android+OR+mobile+OR+composition): #202 (IME target position, 2026-10-09), #190 (composition drawing at cursor, 2026-08-25), #120 (Korean/CJK composition not captured, 2026-01-25), issue #119 (Korean IME not working), #192 (`attachCustomKeyEventHandler` inverts the xterm return-value contract, which "silently swallows all input"). VERIFIED. All are open; none address autocorrect rewrites or Gboard.
- The upstream effort targets "make beforeinput/IME not drop text". It does not target rewrite semantics or a trace-replay test corpus.
- Verdict: **Not recommended** to wait on or depend on it. Track PR #110 and a release as a free partial improvement, and re-run our replay corpus against any newer version.

### 1c. Codeman PR #65 (orphaned-input patch)

- Source: https://github.com/Ark0N/Codeman/pull/65/files. One file (`src/web/public/terminal-ui.js`, +25/-1), merged 2026-04-18. VERIFIED.
- Technique: touch devices only. A listener on `.xterm-helper-textarea` tracks `compositionstart`/`compositionend`. It records a timestamp on handled keydowns. On an `input` event with `inputType === 'insertText'` outside composition, it skips if a normal keydown was handled within 50 ms. Otherwise, in a microtask, if the textarea is still non-empty (xterm did not consume it), it forwards the char via `terminal._core.coreService.triggerDataEvent(data, true)` and clears the textarea.
- Gaps: it covers only orphaned `insertText` (e.g. the symbol-keyboard `/`). It has no handling for `insertReplacementText`/autocorrect/suggestion picks, swipe words, or backspace diffs. It uses a private xterm API (`_core.coreService`), is xterm-only, uses a 50 ms timing heuristic, and has no tests in the diff. License not shown on the page.
- Verdict: **Viable only as a reference** for the orphaned-`input` fallback and its 50 ms dedupe idea. **Not recommended** to port as-is.

### 1d. CodeMirror 6 input handling

- `@codemirror/view` `domobserver.ts` (https://raw.githubusercontent.com/codemirror/view/main/src/domobserver.ts) uses a `MutationObserver` and `readChange()` to turn DOM mutations into a document change. For Android it has `delayAndroidKey()` / `delayedAndroidKey`, which defers key handling by one `requestAnimationFrame` so that `flush()` can resync first. Enter overrides a pending key. On Chrome Android >= 126 with `window.EditContext` it uses an `EditContextManager`. VERIFIED.
- This is the most mature public handling of Gboard quirks. It is MIT, maintained, and already a dependency of this repo (`@codemirror/view`). But it is tied to CodeMirror's document model, `EditorView` and DOM, and is not exposed as a reusable input module. Wrapping a hidden `EditorView` as an input proxy is possible in principle, but it brings a heavy state/view dependency, an offscreen document to keep in sync, and requires mapping doc changes to terminal bytes (INFERRED).
- Verdict: **Not recommended** to adopt. **Recommended as design reference**: the one-rAF defer of keys, flush-on-boundary, delta from a snapshot, and EditContext as a progressive path on Chrome >= 126 Android.

### 1e. EditContext

- MDN (https://developer.mozilla.org/en-US/docs/Web/API/EditContext_API): experimental, "not Baseline", limited availability. The compat table was not in the fetched text, so Chrome Android support is not confirmed from MDN. W3C draft: https://www.w3.org/TR/edit-context. VERIFIED partial.
- The CodeMirror source above shows that CodeMirror uses EditContext on Android Chrome >= 126, which is the practical evidence it ships there. VERIFIED (via source).
- Polyfills: none found; a search returned nothing relevant (UNVERIFIED that none exist). A polyfill can't give soft-keyboard access anyway, since the soft keyboard is a browser/OS surface (INFERRED).
- Verdict: **Viable as a later progressive enhancement** behind the same seam (same event-to-bytes interface). **Not recommended** as the primary path (no Firefox/Safari, experimental).

### 1f. Others

- hterm, VS Code terminal, Termux web, Blink, ttyd/wetty, Lexical/ProseMirror: a targeted search found no documented Gboard fix in hterm/ttyd/wetty. UNVERIFIED for all of these; not read in depth. jquery.terminal issue #39 (https://github.com/jcubic/jquery.terminal/issues/39) confirms the underlying cause: Chrome Android does not fire `keypress` during composition, so Android needs its own detection and character injection. VERIFIED (search snippet).
- Workaround class seen in the wild (xterm.js #3600 comments; mobux `input-bar.js` at https://docs.rs/crate/mobux/0.12.1/source/web/static/input-bar.js): a separate visible input bar or a virtual keyboard that bypasses the renderer's textarea. That is a UX change, and the owner said the toolbar redesign is out of scope.
- Verdict: **Not recommended** as dependencies. ProseMirror (`view/src/domchange`) and Lexical use the same snapshot-and-diff-DOM technique as CodeMirror, as design references only (INFERRED; not fetched).

### 1g. Diff libraries for rewrites

| Package | Version | License | Notes |
|---|---|---|---|
| `diff` (jsdiff) | 9.0.0 | BSD-3-Clause | 0 runtime deps, 615 KB unpacked (https://registry.npmjs.org/diff/latest) |
| `fast-diff` | 1.3.0 | Apache-2.0 | 0 runtime deps (https://registry.npmjs.org/fast-diff/latest) |
| `diff-match-patch` | not checked | Apache-2.0 (INFERRED) | unmaintained (INFERRED) |

- The needed operation is narrow. Given composition-region text `before` and `after`, compute the common prefix, the common suffix, then emit N x DEL (0x7f) for the removed chars and the inserted text. That is O(n), about 20 lines, and fully defined by the contract. It is *not* a general diff. Terminal semantics want exactly that "single contiguous edit" shape, and a Myers-style multi-hunk diff would produce edit sequences (and cursor jumps) that a TUI input box handles worse (INFERRED).
- Verdict: **Not recommended** to add `diff` or `fast-diff`. They don't offer cursor-aware or grapheme-aware semantics either, so we need our own grapheme handling (`Intl.Segmenter`) in any case (INFERRED).

## 2. SaaS / managed

No hosted terminal IME or soft-keyboard service was found, and this is not the kind of component that is sold (INFERRED; the searches above surfaced none). A managed web terminal would just embed xterm.js, and so would hit the same bug. **Not applicable.**

## 3. LLM-generated bespoke code vs a battle-tested library

When custom code is justified here:
- The event contract (Gboard/Samsung emit non-uniform `beforeinput`/`input`/`composition*` sequences) is specific to this problem and not captured by any library. The output contract is "bytes for a terminal", also specific to us. The core pieces are small, pure functions, so they are testable in isolation.
- The renderer-agnostic requirement rules out every renderer-internal fix (Codeman, upstream PRs).

When it is reckless: untested heuristics (timing windows like Codeman's 50 ms) validated only by "it worked on my device", and reliance on private renderer internals. The risk is silent byte loss or duplication, which is exactly the failure being fixed. LLM-written state machines tend to pass the happy path and fail on event-order variants, which is why PR #110 in ghostty-web needed tests for both orders of paste/`beforeinput` and `compositionend`/`beforeinput` (VERIFIED from the PR text above).

Risk offsets (design, INFERRED; none of this is installed yet):
1. Pure core: `reduce(state, event) -> {state, bytes[]}` with no DOM. The DOM adapter only translates events.
2. Trace corpus: JSON event traces (type, inputType, data, isComposing, keyCode, selection, textarea value) each paired with expected final bytes. Sources: hand-synthesized from published sequences (xterm.js #3600, Codeman PR, CodeMirror's Android notes), then real recordings from the owner's Gboard/Samsung via a dev-only recorder. Synthesized traces alone are insufficient; label them as such.
3. Invariants as `fast-check` properties (already a devDependency): replaying a trace in any permitted event-order permutation (e.g. `compositionend` before/after `beforeinput`, as in PR #110) yields identical bytes; the terminal-side "virtual input line" after applying emitted bytes equals the final textarea value; no bytes emitted outside the composition region.
4. A model terminal in the test (apply DEL/text to a string buffer) to check the rewrite maths end to end.
5. Playwright with Chromium mobile emulation plus CDP `Input.imeSetComposition`/`Input.insertText` for real IME paths (INFERRED to be available; verify in design). Emulation is not Gboard, so owner phone acceptance remains a gate.
6. A kill switch and anomaly counters (already in requirements) to catch drift in production.

## 4. Fork / adapt options

| Option | Verdict | Reason |
|---|---|---|
| `bun patch` of `@xterm/xterm` (CompositionHelper) | Not recommended | Patches a minified/bundled dist, reapplied each upgrade; xterm-only; `patchedDependencies` precedent exists but only for a transitive pin |
| `bun patch` of `ghostty-web` 0.4.0 | Not recommended | Pinned exact, so a patch is low-churn, but it couples us to its internals and fixes only one renderer. Would be discarded when the PR #110 release lands |
| Upstream PR to xterm.js (#3600 / `CompositionHelper`) | Viable follow-up | Right for the ecosystem, but no timeline control (maintainer deprioritised it); out of scope per requirements |
| Upstream PR to ghostty-web | Viable follow-up | PRs sit open for months (#120 since Jan 2026); out of scope per requirements |
| Wrap from outside (our layer owns the hidden textarea, renderer exposes only "send bytes") | **Recommended** | Renderer swaps cannot break phone typing. Needs each renderer's own textarea/keydown path disabled or ignored on touch; this seam is the design question for phase 3 |

For the outside wrapper the layer should not call `triggerDataEvent` or any private API; it should feed the same `onData` sink the renderers already use (the existing `sendInput` path with batching), per the requirements.

## Dependencies this approach adds

None at runtime. Optionally for tests, nothing new: `fast-check ^4.10.2`, `@testing-library/user-event ^14.5.2`, `jest-environment-jsdom ^30.2.0` and `@playwright/test ^1.57.0` already exist.

If the design review decides a library diff is wanted anyway (not recommended), the candidates are `diff@9.0.0` (BSD-3-Clause) or `fast-diff@1.3.0` (Apache-2.0). Both are 0-dependency. Add them in `web-app/` with `bun add --exact`, no `trustedDependencies` entry (no install scripts known; INFERRED, not checked), and the version must be older than 3 days to satisfy `minimumReleaseAge`. Note: existing deps mostly use `^` ranges, while `ghostty-web` is an exact pin. The "exact pins" rule is from the task brief and not verified in the repo; confirm against `docs/how-to/use-bun-in-web-app.md`.

## Open items

- Confirm whether ghostty-web main (post-#110) removes the unconditional `preventDefault` on `beforeinput`; check by building from main in a scratch dir, never in the repo.
- Confirm Chrome Android EditContext support via caniuse/MDN compat table before scoping it as a progressive path.
- hterm, VS Code terminal, Termux, Blink, Lexical, ProseMirror were not read in depth.
