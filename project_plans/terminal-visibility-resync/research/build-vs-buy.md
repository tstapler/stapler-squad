# Build vs. buy: terminal visibility/focus resync

Research question: for the two sub-problems in this fix (debounce hook, visibility/focus
detection) and the broader resync mechanism, is there an OSS solution worth adopting
instead of hand-rolling? "No new npm dependency" is stated as a preference in
`requirements.md` — this doc validates that preference rather than assuming it.

## Method

- Read the only `package.json` in the repo (`web-app/package.json`) — this is a
  single-package repo, not a monorepo with multiple frontend packages, so there is
  nowhere else a hook library could already be declared.
- Searched `web-app/pnpm-lock.yaml` (the sole lockfile) for `use-debounce`,
  `lodash.debounce`/`lodash/debounce`, `usehooks-ts`, `react-use`, `ahooks`.
- Read the current `useDebounce.ts` implementation and its only consumer.
- Checked the installed `@xterm/addon-*` packages against the official xterm.js addon
  catalog for any resync/visibility/buffer-validation capability.
- Sized the surrounding hooks (`useTerminalStream.ts`, `useTerminalFlowControl.ts`,
  `TerminalOutput.tsx`) to judge whether this is glue code or a new subsystem.

## 1. Debounce hook — is a library already present?

**No debounce library is a direct or transitive runtime dependency.** Searched the
full `pnpm-lock.yaml`:

- `use-debounce`, `usehooks-ts`: zero matches anywhere in the lockfile.
- `lodash.debounce`: present, but **only** as a transitive dependency of
  `@babel/helper-define-polyfill-provider` (pulled in by `@babel/preset-env`/Storybook
  build tooling) — a build-time Babel helper, never bundled into client code, and not
  hoisted/declared in `web-app/package.json`. Under pnpm's strict `node_modules`
  layout, `import debounce from 'lodash.debounce'` in app source would not even
  resolve without adding it as a direct dependency — it's invisible to app code today.

So the premise "bundle size is already paid" does **not** hold for lodash.debounce —
using it would still mean adding a new direct dependency, just one whose transitive
cousin happens to already sit in `node_modules` for unrelated dev tooling.

**Current implementation** (`web-app/src/lib/hooks/useDebounce.ts:31-56`,
`useDebouncedCallback`) has exactly the bug requirements.md describes: the timer id is
`useState`-backed, so `clearTimeout(timeoutId)` on the second same-tick call reads the
stale pre-update `timeoutId` (React batches the `setTimeoutId` call, it doesn't apply
synchronously), so the first timer is never cleared and both fire. It's also not
memoized — a fresh closure is returned every render, defeating any consumer that relies
on referential stability (e.g. putting it in a `useEffect` dependency array), which
matters for AC1 (dedupe visibilitychange+focus in one tick) and the watchdog wiring in
AC5.

**`useDebouncedCallback` currently has zero consumers in the codebase** (`grep` finds
only its own definition; `logs/page.tsx` uses the separate value-debouncing
`useDebounce`, not `useDebouncedCallback`). This fix is the function's first real use
— so any latent bugs beyond the described one have never been exercised in production.

**Recommendation: fix in place, do not swap to a library.** The fix is a ~10-line
change (useRef for the timer id + `useCallback`/`useMemo` for a stable return), it's
already the sanctioned/described root-cause fix in requirements.md, and it doesn't
warrant a new dependency (`use-debounce` is ~2KB but still a new supply-chain entry,
new devDependency lockfile churn, and a new API surface to learn) for a problem that's
fully solved by correcting two well-understood React idioms (ref vs. state for
mutable-non-rendered data, `useCallback` for referential stability). If more call sites
adopt debouncing later and the hand-rolled hook accumulates edge cases, revisit — but
today there's exactly one bug, one fix, one consumer.

## 2. Visibility/focus/idle detection — existing OSS hook already present?

**No utility-hook library (`react-use`, `ahooks`, `usehooks-ts`) is present anywhere in
the monorepo.** The repo has only one `package.json` (`web-app/package.json`) — there
is no separate frontend package where such a library could be hiding. Full-repo search
for `react-use`, `ahooks`, `usehooks-ts` (including all `*.json`/`*.yaml`/`*.lock`
files) returns zero hits outside of unrelated `@radix-ui/react-use-*` internal hook
names (Radix's own internal `useCallbackRef`/`useLayoutEffect` helpers, not a
general-purpose visibility hook).

Even setting aside "not present," the value proposition of adopting one here is weak:
`react-use`'s `useDocumentVisibility` and `ahooks`' `useDocumentVisibility` both just
wrap the raw `document.visibilitychange` event and return the current
`document.visibilityState` — they do **not** debounce, do **not** dedupe
same-tick multi-event races, and do **not** know anything about `window focus` (a
second event that requirements.md explicitly requires be coalesced with
`visibilitychange`). Using one would still require hand-writing the debounce, the
dedupe-in-same-tick logic, the connected/disconnected branching, and the stall
watchdog on top — i.e. it would replace ~3 lines of raw event-listener boilerplate
with a new dependency and save none of the actual novel logic. The hard/valuable part
of this fix (debounce dedupe, connected vs. disconnected branching, stall watchdog,
focus-preservation) is 100% bespoke to this app's RPC/reconnect model and isn't
something any general-purpose visibility hook provides.

**Recommendation: hand-roll the listener**, exactly as scoped in requirements.md. A
library buys nothing here.

## 3. xterm.js addon capability — does resync/buffer-validation already exist?

Installed addons (`web-app/package.json`): `@xterm/addon-fit`, `@xterm/addon-search`,
`@xterm/addon-serialize`, `@xterm/addon-web-links`, `@xterm/addon-webgl`, on
`@xterm/xterm ^6.0.0`.

Cross-checked against the official xterm.js addon catalog (xtermjs/xterm.js repo,
`addons/` directory): `addon-attach`, `addon-canvas`, `addon-clipboard`, `addon-fit`,
`addon-image`, `addon-ligatures`, `addon-progress`, `addon-search`, `addon-serialize`,
`addon-unicode11`, `addon-unicode-graphemes`, `addon-web-links`, `addon-webgl`. None of
these — including the ones already installed — provide a "resync on visibility" or
"validate buffer against source of truth" capability:

- `addon-serialize` *produces* a serialized snapshot of the current buffer (used
  elsewhere in this repo for the session-list preview hook per requirements.md) — it
  has no concept of comparing that snapshot against the real pane or detecting
  staleness.
- `addon-attach` is the closest thing to a "wire the terminal to a live
  transport" addon, but it's a dumb raw-WebSocket pipe (write bytes in, read bytes
  out) with no framing/RPC awareness — irrelevant here since this app already has its
  own ConnectRPC-based `TerminalStreamManager` handling structured
  `TerminalData_Output` messages, not raw attach-addon semantics.
- No addon has any concept of `document.visibilitychange`, tab backgrounding, or
  round-trip staleness detection — that's inherently an application-level concern
  (xterm.js only renders whatever bytes it's given; it has no way to know the
  "real" pane state on the server).

**Recommendation: no addon applies.** The resync mechanism (force pane re-capture via
RPC, clear+repaint on the client) has to be application logic; xterm.js's job stops at
rendering the stream it's handed.

## 4. Overall recommendation: is any external dependency justified?

**No.** Sizing the actual surface area:

- `useTerminalFlowControl.ts` (330 lines) already implements `requestFullResync`,
  `markResyncComplete`, `markPaneResponseReceived`, and the tracking refs — this fix
  wires ~3 already-built functions through `useTerminalStream.ts`'s return value.
- `useTerminalStream.ts` (474 lines) already has an analogous (if narrower)
  `visibilitychange`/`online` listener behind `NEXT_PUBLIC_RECONNECT_V2` — the new
  handler is an incremental sibling to code that already exists in the same file.
  `connect()`/`disconnect()` are pre-built.
- The novel code is: one small debounced event listener (~20-30 lines), one stall
  watchdog `setTimeout`/`clearTimeout` pair (~15-20 lines), a handful of new returned
  refs/functions threaded through an existing hook interface, and the one-hunk
  `useRef` fix to `useDebounce.ts`. Total new logic is well inside the 50-150 line
  estimate in the task description.

This is thin glue code stitching together pre-built primitives in the same file/hook,
not a reusable general-purpose utility (the kind of thing a library would earn its
keep on). None of the three candidate replacement points (debounce, visibility
detection, xterm resync) have a library on the table that is both (a) actually present
in the dependency tree today and (b) would reduce the amount of bespoke logic that
still has to be written. The "no new dependency" preference in requirements.md is
correct as stated, not merely a default assumption — validated, not just accepted.

## Summary table

| Sub-problem | OSS candidate | Present in repo? | Would it reduce bespoke code? | Verdict |
|---|---|---|---|---|
| Debounce hook | `use-debounce`, `usehooks-ts` | No (zero hits) | Marginally — still hand-rolled logic elsewhere | Fix in place |
| Debounce hook | `lodash.debounce` | Transitive only, via Babel build tooling; not usable from app code without adding a direct dep | No — same as above, plus wouldn't actually be free | Fix in place |
| Visibility/focus | `react-use`/`ahooks` `useDocumentVisibility` | No (zero hits repo-wide) | No — doesn't debounce, dedupe, or handle `focus`/connect state | Hand-roll |
| xterm resync | Any `@xterm/addon-*` | Several installed, none applicable | No — no addon touches visibility or buffer validation | Hand-roll (app-level RPC) |
| Overall | Any dependency | — | No — task is glue code over pre-built primitives | Build, no new dependency |
