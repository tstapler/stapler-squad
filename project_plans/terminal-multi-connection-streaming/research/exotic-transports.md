# Research: Exotic Browser Transports (IWA Direct Sockets, WASM Terminal Clients)

**Phase**: 2 (research) for `terminal-multi-connection-streaming`
**Question being answered** (from `requirements.md`'s Open Questions / Rabbit Holes): are Chrome IWA
Direct Sockets or WASM-based terminal clients relevant to this product today?

**Bottom line: not applicable now.** Neither is ruled in even as a follow-on transport implementation
under the new hub/transport interface. Details and the specific trigger conditions that would change
this are below.

## 1. Chrome Isolated Web Apps (IWA) Direct Sockets

### What it is

Direct Sockets (`TCPSocket`, `TCPServerSocket`, `UDPSocket`) is a raw TCP/UDP API for the web platform.
It is deliberately **not** available to normal web pages — the WICG explainer and Chrome's own docs
state it can't be made safe enough for the standard "drive-by web" security model
([Direct Sockets | IWA | Chrome for Developers](https://developer.chrome.com/docs/iwa/direct-sockets),
[WICG/direct-sockets explainer](https://github.com/WICG/direct-sockets/blob/main/docs/explainer.md)).
It is gated entirely behind **Isolated Web Apps** (IWA), a distinct application model layered on top
of the web platform — not a browser tab, not a PWA, not a normal `https://` origin.

### What it requires (packaging/distribution)

- The app must be built as a **signed Web Bundle** (`.swbn`), with a manifest declaring an explicit
  `permissions_policy` block enabling `direct-sockets` and `cross-origin-isolated`
  ([Chrome for Developers](https://developer.chrome.com/docs/iwa/direct-sockets)) — without both keys
  set, the socket constructors reject immediately with `NotAllowedError`.
- Installation is **not** "open a URL." Even for development, it requires enabling
  `chrome://flags/#enable-isolated-web-apps` (and `#enable-isolated-web-app-dev-mode`), then installing
  through `chrome://web-app-internals` via either a signed bundle install or a "Dev Mode Proxy" pointed
  at a local dev server ([ChromeOS.dev getting-started guide](https://chromeos.dev/en/tutorials/getting-started-with-isolated-web-apps)).
  Production distribution means shipping a signed bundle and having the user install it outside the
  ordinary web-navigation flow — closer to sideloading a browser extension or a ChromeOS/enterprise app
  than to visiting a site.
- Chrome/ChromeOS 120+ only; broad general availability (non-flag-gated, cross-platform) is not
  established as of 2026 — the primary published examples (IWA Kitchen Sink, the Telnet Client demo)
  are still framed as developer samples, not shipped consumer products. There is also an enterprise
  policy path (admins can allowlist specific origins for Direct Sockets), which is irrelevant to a
  personal single-operator tool.

### What it would take for this product

stapler-squad is a normal Next.js/React SPA served over plain HTTP(S) from `localhost:8543` or a
self-signed-TLS remote port — a URL a browser tab loads with zero install step. Adopting Direct Sockets
would mean:

1. Repackaging the web-app as a signed IWA bundle (a second build/distribution pipeline alongside the
   existing web build), with its own versioning and signing-key management.
2. Requiring every user (in practice: Tyler, on however many machines) to flip Chrome IWA flags and
   explicitly install the bundle — a one-time but nontrivial, browser-specific onboarding step, and one
   that doesn't work at all in Firefox/Safari.
3. Losing the "just open a URL" property that makes the current deployment model (localhost dev server,
   or a remote port with a self-signed cert) trivially portable across the operator's own devices.
4. Gaining, in exchange: the ability to open a raw TCP socket from the frontend directly to some backend
   — but stapler-squad's terminal streaming problem is not blocked on lacking raw TCP. It already has a
   working transport (WebSocket via gorilla/websocket, soon ConnectRPC bidi per the parent plan) carrying
   a purpose-built framed protobuf protocol. Direct Sockets would only matter if the goal were to speak
   a raw, non-HTTP protocol (e.g., real SSH/Telnet, per Chrome's own example use cases) directly from the
   browser, bypassing the Go server's WebSocket endpoint entirely. Nothing in the requirements doc calls
   for that — the Go server owning the tmux session and framing updates is the whole point of the hub
   design this project is building.

### Verdict

Not applicable. The packaging/distribution burden (signed bundle, flag-gated manual install, browser-specific)
is disproportionate to a single-operator tool whose entire value proposition today is "open a browser tab."
It would also solve a problem this product doesn't have — the bottleneck identified in
`requirements.md` is the server-side lack of a single stream owner and a transport abstraction, not an
inability to speak raw sockets from the browser.

## 2. WebAssembly-based terminal clients

### What "WASM terminal client" actually means (disambiguating)

The phrase is ambiguous and splits into two unrelated ideas — the research question in
`requirements.md` doesn't specify which, so both are addressed:

**(a) A WASM-compiled terminal *emulator* (replacing xterm.js's parser/renderer).** This is the
concrete, real-world case. Examples found:
- **[ghostty-web](https://www.npmjs.com/package/ghostty-web)** (Coder) — compiles Ghostty's native VT
  parser to WASM and exposes an xterm.js-compatible API as a "drop-in replacement." Their stated
  rationale: xterm.js hand-codes every escape sequence and Unicode quirk in JavaScript, whereas
  Ghostty's parser is the same battle-tested native code, giving better RTL/complex-script/edge-case
  escape-sequence fidelity. ~400KB bundle, zero runtime deps.
- **[wterm](https://pyshine.com/Wterm-Terminal-Emulator-Web-Vercel/)** (Vercel Labs) — a Zig-compiled
  WASM escape-sequence parser (~12KB) paired with **DOM rendering instead of canvas**, specifically to
  get native text selection, clipboard, Ctrl+F search, and screen-reader support "for free" from browser
  primitives, plus dirty-row tracking so a `top`/`htop`-style redraw only touches changed rows.
- **[xterm-pty](https://xterm-pty.netlify.app/)** / **wasm-webterm** — these run an actual
  Emscripten/WASI *program* (a shell, a CUI binary) client-side and bridge it to xterm.js's line
  discipline. Not applicable here: stapler-squad's terminal content is tmux output happening on a real
  remote (server-side) session, not a program the browser itself is expected to execute.

**(b) Running transport/protocol logic in WASM.** No evidence found of this being a distinct pattern
in the terminal-streaming space — WebSocket/protobuf parsing is cheap enough in JS/TS that nobody
appears to be compiling that part to WASM for a browser terminal product. This interpretation doesn't
correspond to a real precedent; it's not what "WASM terminal client" means in practice.

### What problem it would actually solve here

Category (a)'s real, cited motivations are **escape-sequence/Unicode edge-case correctness** (ghostty-web)
and **native selection/accessibility/rendering performance for high-churn output** (wterm) — not
transport, not multi-connection coordination, not the resize/capture race this project exists to fix.
stapler-squad's actual open problems per `requirements.md` are: (1) no single owner of a tmux session's
resize/capture, (2) transport hard-coupled to `*websocket.Conn`, (3) no frame batching. A WASM-compiled
terminal emulator changes none of these — it's a client-side rendering/parsing swap that would sit
*downstream* of whatever the hub/transport redesign produces, same as xterm.js does today. It's also not
a novel or exotic integration: swapping the terminal library is a normal frontend dependency change,
not an architectural transport question, and this repo already invested specifically in xterm.js jank
fixes (`project_plans/terminal-jank`, `project_plans/terminal-resize-fit-loop`) that a library swap would
partially re-litigate.

### Verdict

Not relevant to *this* project's scope. If pursued at all, it's an independent, much smaller frontend
question ("should we replace xterm.js with ghostty-web/wterm for rendering fidelity or performance") that
belongs nowhere near the transport/hub redesign — it doesn't touch the server-side streaming
architecture, the transport interface, or the multi-connection race this plan is fixing.

## 3. Recommendation

**Not applicable — do not pursue either as part of this project.** Neither exotic browser transport
solves a problem in scope for `terminal-multi-connection-streaming`, and both carry costs
disproportionate to a single-operator, no-app-store-distribution, "open a browser tab" tool:

- **IWA Direct Sockets**: revisit only if a future requirement needs the *browser itself* to speak a
  raw non-HTTP protocol directly to something other than this product's own Go server — e.g., a
  from-scratch SSH/Telnet client feature, or bypassing the server entirely for a cross-host scenario.
  Given `ADR-002`'s Workspace Host Registry already plans cross-host streaming through a Go-owned socket
  layer (per `requirements.md`'s Out of Scope section), that trigger looks unlikely to materialize; if it
  ever does, revisit distribution cost against whatever IWA's install story looks like at that time
  (flag-gating and enterprise-only policy paths may loosen).
- **WASM terminal clients**: not an architecture question at all for this product — track it, if ever,
  as a standalone frontend improvement (rendering fidelity/performance) fully decoupled from the
  hub/transport work, triggered only by a concrete, observed xterm.js limitation (e.g., a real
  escape-sequence/Unicode rendering bug xterm.js can't fix, or measured rendering performance problems
  under high-throughput output that a batching/coalescing fix on the server side doesn't resolve).

Neither should block or influence Phase 3 planning for the hub/transport design; the parent requirements
doc's own hypothesis ("not now") is confirmed.

## Sources

- [Direct Sockets | Isolated Web Apps (IWA) | Chrome for Developers](https://developer.chrome.com/docs/iwa/direct-sockets)
- [Isolated Web Apps (IWA) | Chrome for Developers](https://developer.chrome.com/docs/iwa/introduction)
- [WICG/direct-sockets explainer.md](https://github.com/WICG/direct-sockets/blob/main/docs/explainer.md)
- [Getting started with Isolated Web Apps | ChromeOS.dev](https://chromeos.dev/en/tutorials/getting-started-with-isolated-web-apps)
- [ghostty-web (npm)](https://www.npmjs.com/package/ghostty-web)
- [wterm: A High-Performance Web Terminal Emulator Built with Zig and WASM](https://pyshine.com/Wterm-Terminal-Emulator-Web-Vercel/)
- [xterm-pty](https://xterm-pty.netlify.app/)
- [wasm-webterm (GitHub)](https://github.com/cryptool-org/wasm-webterm)
