# Research: UX for terminal-multi-connection-streaming

Scope note per requirements.md's Appetite: this item is a backend/architecture redesign (hub +
transport interface). The UI surface is intentionally narrow — a connection-count/presence
signal and an error state or two in `XtermTerminal.tsx`/`TerminalOutput.tsx`. This doc sizes that
surface; it does not propose a presence-avatar system or anything comparable to Google Docs'
multiplayer UI, which would be disproportionate to a single-operator tool.

## 1. Comparable UX patterns

| Tool | How it signals "someone/something else has a view" | Why it works |
|---|---|---|
| **tmux multi-client attach** | Nothing, by default. If two clients attach to the same session at different terminal sizes, tmux clamps the shared window to the *smallest* attached client's size (`aggressive-resize` off, the default) — bigger clients see empty padding, not their own layout. There is no banner; the size-shrink itself *is* the signal. `tmux list-clients` is the only way to see who's attached, and it's opt-in (you have to run it). | This is the closest architectural precedent to this repo's problem (`session/scanbuf`, control-mode capture-pane) and directly informs the "Open Questions" resize-negotiation decision in requirements.md. It works for tmux's audience (a person who already knows tmux internals) but is a bad model for a GUI app: silent size-clamping with no explanation is confusing to someone who didn't choose to open a second terminal on purpose (e.g., the browser tab they forgot they left open). |
| **VS Code Live Share (terminal sharing)** | A read-only terminal shown to guests renders as a plain scrollback view sized to *their own* pane — Live Share does not force the guest's terminal to match the host's dimensions. The host's status bar shows a persistent "Live Share" indicator with a participant count while a session is active. | Decouples "who is watching" (a lightweight, always-visible count) from "what size is the underlying process" (each viewer gets their own rendered view, not a shared PTY size). This maps directly onto the resize-negotiation question: don't force one dimension onto all subscribers if it can be avoided — reflow/pad instead. |
| **Google Docs presence (avatars + colored cursors)** | Every collaborator gets a persistent, always-visible avatar; the moment someone's viewport differs from yours, nothing forces your document to reflow to fit them — each user's document rendering is independent of others' viewports. | The strongest lesson to *not* import here: full presence UI (avatars, per-user color, live cursor tracking) is built for genuine multi-user collaboration. This repo has exactly one human operator; the "other viewer" is almost always that same person in a second tab, or a passive IDE terminal. A Docs-style presence system would be over-engineering for the actual job (see §5). |
| **Zellij** | Native multiplayer sessions show a small floor/ceiling indicator of concurrent users in the status bar and supports independent per-user "focus" panes, but resizing is still session-wide (Zellij, like tmux, has one PTY grid shared by all attached clients). No collision-avoidance UI beyond the status bar count. | Reinforces that even "multiplayer-native" terminal multiplexers don't solve the resize-negotiation problem visually — they solve it structurally (one broker owns layout) and surface only a lightweight count. That maps onto this repo's actual plan (a hub owning resize) rather than a UI trick. |

**Takeaway for this repo:** the strongest precedent is "structural fix + lightweight count," not
"rich presence UI." None of the tools above show a rich avatar/cursor system for terminal
sharing specifically — that pattern only shows up for document/text co-editing (Docs), which
isn't this repo's use case (one operator, multiple views of the same read-mostly PTY).

## 2. User mental model: same operator, two views

The realistic scenarios named in requirements.md are:
- The same operator with the same session open in two browser tabs.
- The same operator with a browser tab open **and** an external IDE terminal (ssq-mux) attached
  to the same session.
- A rapid reconnect after a server restart, which today's `420584566` WARN exists to detect —
  transient, not really "two viewers" from the user's perspective even though it looks like one
  to the hub.

None of these is a second *person*. That reframes the design question from "how do we show
Alice that Bob is watching" to "how do we tell an operator that they, themselves, have two
windows open on the same thing" — closer to a **stale-tab / duplicate-session warning** (a
pattern users already know from apps like Slack or Figma: "You have this open in another tab")
than to a collaboration-presence pattern.

**What should happen if the operator resizes one tab?** Given tmux control-mode's actual
constraint (one pane, one set of dimensions — confirmed as an open Feasibility Risk in
requirements.md, not yet verified), the honest answer is: **the other tab's rendering can
change**, and pretending otherwise would be misleading. The expectation to design for is not
"nothing visibly changes elsewhere" (technically hard to guarantee and arguably not even
desirable — a too-small shared pane hurts both viewers) but:

1. **The change should be intentional-looking, not garbled.** The bug this whole item exists to
   fix (`session/services/connectrpc_websocket.go`'s uncoordinated resize/capture race) produces
   *overlapping, corrupted* output — that's the actual UX complaint, not "the size changed." A
   clean, correctly-redrawn pane at a new (possibly clamped) size is an acceptable outcome even
   if it's not the size the second tab asked for.
2. **The second tab should be able to tell *why*** its size doesn't match what it requested,
   the moment it notices. This is squarely the connection-count/presence indicator's job (see
   §4): "this session has another active connection" is sufficient context for an operator who
   already knows they have two tabs open — they don't need a blow-by-blow of the resize
   negotiation.
3. **Whatever resize-negotiation model Phase 3 picks (authoritative-subscriber vs.
   smallest-common-size)**, the UI-relevant consequence is the same either way: a tab whose
   requested size loses the negotiation needs a small, non-blocking explanation, not a silent
   shrink. This is a **content note for the Phase 3 architecture decision**, not something this
   research settles — the UX doesn't change materially between the two models, only which tab
   the explanation shows up on.

## 3. Accessibility

This repo already has an established, referenceable pattern rather than needing a new one:

- **`role="alert"` + implicit assertive announcement** — used in
  `web-app/src/components/DeepLinkErrorBanner.tsx` for the four "genuine dead end" failure
  reasons (deleted/archived/malformed/version-mismatch), and again in
  `TerminalOutput.tsx:1797` for the existing hard-failed-connection banner
  (`<div className={styles.hardFailedBanner} role="alert">`). Reserve this for states that
  actually block the user (stream can't be established at all).
- **`role="status"` + `aria-live="polite"`** — used in `DeepLinkErrorBanner.tsx` for the two
  cross-host, non-blocking cases (unreachable/not-registered host), and already the pattern for
  `TerminalOutput.tsx`'s own reconnecting banner (`TerminalOutput.tsx:1789-1791`,
  `aria-label="Reconnecting"`) and its resizing overlay (`TerminalOutput.tsx:1822-1823`,
  `aria-label="Terminal resizing"`). `BacklogItemDetail.tsx:1290-1292`'s
  `copy-status-announcement` region is the same idiom applied to a transient one-shot
  confirmation ("Copied!") rather than a standing banner — useful precedent if a presence change
  is announced as a brief, dismissable toast instead of a persistent element.
- **Recommendation for the connection-count indicator**: `role="status"` +
  `aria-live="polite"`, matching the existing reconnecting-banner and resizing-overlay
  convention in this same file — a connection-count change is informational, not something that
  should interrupt a screen-reader user mid-task the way `role="alert"` would. It should **not**
  announce on every mount (only on count *changes*, so opening the tab that already has 1
  connection doesn't fire an announcement before the user has done anything) — same debounce
  discipline `InputDropBadge.tsx`'s episode-coalescing pattern already applies to a
  similarly transient, could-be-noisy signal.
- The icon-only visual treatment used in `DeepLinkErrorBanner.tsx:127-129`
  (`<span aria-hidden="true">`) is the right model if the count indicator gets an icon: hide the
  glyph from the accessibility tree and let the visible/announced text (e.g. "2 connections
  active") carry the meaning.

## 4. Error states

Two distinct failure modes need distinguishable copy, following the same "don't lump distinct
failure reasons into one generic banner" discipline `DeepLinkErrorBanner.tsx` established for
deep-link failures (its AC4: deleted vs. archived must be visually/textually distinct even
though both are "alert"-severity):

**(a) Hub/stream can't be established at all** (new failure mode introduced by this
architecture — e.g. the hub itself fails to start, or the transport registration fails before
any subscriber attaches). This is a genuine dead end for *this* connection attempt and should
reuse the existing `hardFailedBanner`/`role="alert"` pattern already in `TerminalOutput.tsx:1797`
rather than inventing new banner chrome — from the user's perspective it's the same "connection
is dead, here's Retry" experience as today's hard-failure path, regardless of whether the
underlying cause is a control-mode failure (today) or a hub-start failure (post-redesign). The
error should not need to explain hub/transport internals to the user; "Connection lost — Retry"
already covers it, same as today.

**(b) This tab's resize/rendering looks "wrong" because another tab or connection is also
attached.** This is not an error — nothing failed — so it should not use `role="alert"` or the
red/blocking treatment. It's better modeled as a **contextual note attached to the existing
connection-count indicator** than as a separate banner: when a resize request doesn't take
effect at the requested size, and the indicator shows more than one active connection, the
indicator's tooltip/expanded text is the natural place for "Another connection has this session
open at a different size" — surfaced on demand (hover/tap) rather than as an unprompted
`role="status"` announcement, since it's explaining a *secondary* effect of something the count
indicator already announced (the connection-count change itself). Do not build two independent
live regions announcing overlapping information — that duplicates the DeepLinkErrorBanner
mistake index the "status vs. alert" rationale table exists to avoid (announcing the same fact
twice, once as a fact and once as a consequence, is more noise than signal for a screen-reader
user).

## 5. Job-to-be-done

For a solo developer using this as a daily-driver AI-agent-session monitor, "know when multiple
things are watching your terminal" serves:

- **Functional job**: avoid mistaking corrupted/garbled terminal output for the *agent itself*
  malfunctioning. Today, the resize race produces exactly this confusion — a user staring at
  scrambled tmux output has no way to tell whether their Claude session crashed or two browser
  tabs collided. The connection-count signal directly resolves that ambiguity in the same way a
  "reconnecting" banner already resolves "is the agent stuck or is the network down."
- **Emotional job**: reduce the low-grade dread of "did I just lose my agent's output/state,"
  which matters more here than in a typical multi-user app because a stapler-squad session often
  represents unattended, in-progress agent work the user cannot easily reproduce. A calm,
  factual "2 connections active" is reassuring in exactly the way a vague garbled screen is not.
- **Social job**: none, in the traditional multi-user-presence sense — there's no second person
  to coordinate with. The closer analogy is the "you're logged in elsewhere" pattern (Slack,
  banking apps): it's about **self-coordination across the operator's own devices/windows**, not
  collaboration. This is the reason a Google-Docs-style presence system is the wrong reference
  model even though it was worth ruling out explicitly (§1) — building for a social job that
  doesn't exist here would add UI surface the appetite (requirements.md's "extra-large but
  foundational-only" scope) doesn't budget for.

## Summary for planning

- Keep the UI surface to: (1) a small, `role="status"`/`aria-live="polite"` connection-count
  indicator near the terminal chrome, changes-only announcement; (2) reuse the existing
  `hardFailedBanner`/`role="alert"` pattern for a hub-can't-start failure, no new banner type;
  (3) fold "another connection caused this resize" into the count indicator's expanded/tooltip
  state rather than a second live region.
- Whatever resize-negotiation model Phase 3 picks, don't design toward "no visible change in
  other tabs" as a goal — that's not achievable given tmux's single-pane-dimensions constraint
  (pending the Feasibility Risk verification) and isn't what users need; they need a correctly
  redrawn pane plus an honest, low-friction explanation, not size invisibility.
