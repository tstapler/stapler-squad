# Build vs. Buy: OSC Title/Progress Parsing

## Context

This repo already contains **three independent, hand-rolled implementations**
of ANSI/OSC escape-sequence scanning, all converged on the same termination
rules (BEL `\x07` or ST `ESC \` for OSC; final byte `0x40-0x7E` for CSI):

| File | Scope | Notes |
|---|---|---|
| `pkg/ansi/csi.go` | CSI only (`CSIFinalByteClass`, `StripCSI`, `IsCSIFinalByte`) | Single source of truth for the CSI final-byte range after a bug (BUG-025) where `[a-zA-Z]`-only termination was independently reimplemented and independently wrong across `session/detection`, `session/detection/ratelimit`, `session/tmux`, `server/services`, `pkg/analytics`. |
| `pkg/analytics/escape_code_parser.go` (807 lines) | Full VT parser: CSI, OSC, DCS/PM/APC/SOS, charset designation | `parseOSC` (lines 450-483) scans for BEL or ST, caps unterminated scans at 65536 bytes, already extracts/redacts OSC payloads (`extractOSCCommand`, `redactOSCPayloads`, `describeOSC`) for the analytics/telemetry pipeline. |
| `server/terminal/escape_scan.go` (118 lines) | Byte-count scanner mirroring the analytics parser's rules exactly (`scanEscapeSequence`, `scanCSI`, `scanUntilTerminator`) | Comments explicitly cite `pkg/analytics/escape_code_parser.go` as the rules source; same 65536-byte unterminated-scan cap (`maxUnterminatedScan`), same "incomplete sequence at end of buffer → treat as consumed" behavior. Currently only exercised by its own test file — no wired-in caller found (`grep -rl scanEscapeSequence` outside the package itself is empty), suggesting either dead/staged code or a landing spot for exactly this kind of feature. |

And the regex the requirements target directly:

- `session/detection/detector.go:129` — `ansiStripRegex` — `\x1b\][^\x07]*\x07` for OSC, alongside a CSI branch and a charset-designation branch, used only to **strip** OSC content before text-pattern matching, not to capture it.

This is strong direct evidence for how this codebase has already answered the
"hand-roll vs. library" question for this exact escape-sequence family, three
times, independently, over time — always landing on a small byte-level or
regex scanner, never an external terminal-emulation library.

## 1. Existing OSS libraries

**Search of `go.mod` (application dependencies):** no terminal-emulation or
ANSI/OSC parser library is a **direct** dependency. `golang.org/x/term`
(terminal I/O mode control — raw mode, size — not sequence parsing) is the
only terminal-adjacent direct dependency.

**Indirect/transitive hits, checked with `go mod why`:**
- `github.com/Azure/go-ansiterm` and `github.com/moby/term` — both pulled in
  transitively through `github.com/bufbuild/buf` (the protobuf codegen CLI,
  itself a build-time tool dependency) → `docker/docker/pkg/jsonmessage` →
  `moby/term` → `go-ansiterm`. These are Docker's terminal-output-formatting
  stack, not something this repo's PTY/session code imports or would want to;
  pulling them into `session/detection` would create an inappropriate
  dependency on Docker's console-output internals for an unrelated feature.

**Broader Go ecosystem candidates** (not present in go.mod, would be net-new):
- `github.com/gdamore/tcell` — full terminal UI/emulation library (input,
  screen buffer, cell rendering). Massive overkill for parsing one escape
  sequence family; pulls in a screen model this feature doesn't need.
- `github.com/hinshun/vt10x`, `github.com/liamg/tml`, or `github.com/
  charmbracelet/x/vt` (Bubble Tea's vt package) — real VT100/VT220 emulators
  with full screen-buffer state. These solve a superset problem (rendering a
  virtual terminal) when the requirement is narrowly "extract the payload of
  one OSC subtype and detect two characters within it."
- No maintained, narrowly-scoped Go package exists that does *just* "extract
  OSC title/progress payloads from a byte stream" — the ecosystem's OSC
  parsing is bundled inside full VT emulators, which is more surface area
  than this feature needs.

**Assessment:** Adopting a general terminal-emulation library would be
overkill for parsing a single, fixed-format, two-byte-sentinel escape
sequence (`\x1b]0;...\x07`). It would add a new dependency surface (license
review, upgrade churn, larger attack surface for parsing untrusted PTY bytes)
to replace ~10-20 lines of logic the repo has already written correctly
twice (`pkg/analytics/escape_code_parser.go`'s `parseOSC`, `server/terminal/
escape_scan.go`'s `scanUntilTerminator`).

**Verdict: Not recommended.**

## 2. SaaS/managed API

Not applicable — this is purely local, in-process parsing of PTY byte
streams inside the Go server process. There is no network boundary, no data
volume, and no reason to introduce an external service for parsing an escape
sequence. Confirmed and set aside.

## 3. LLM-generated bespoke implementation vs. battle-tested library

The target format is narrow and fixed:
- Sentinel: `\x1b]` (OSC introducer), optional numeric parameter (`0;`, `4;`
  for progress), payload bytes, terminator `\x07` (BEL) or `\x1b\\` (ST).
- The only content this feature needs to extract from the payload: presence
  of a Braille spinner glyph (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏ — already enumerated in
  `session/detection/binaries/claude.go:139`) vs. `✳`.
- Edge cases already identified and handled by the two existing repo
  implementations, not hypothetical: BEL vs. ST termination (both used in the
  wild depending on terminal), unterminated/truncated sequences from
  partial PTY reads (both existing parsers return "0 consumed" / cap the scan
  rather than mis-parsing), and non-OSC escape sequences interleaved in the
  same stream (both parsers dispatch by the byte after ESC before assuming
  OSC).

Given the requirements explicitly scope out OSC progress beyond `\x1b]4;0`
and non-Claude binaries, the correctness surface is smaller than what
`pkg/analytics/escape_code_parser.go` already covers (it also handles
DCS/PM/APC/SOS, charset designation, and CSI final bytes 0x40-0x7E per
ECMA-48 — a documented bug fix, BUG-025, from an earlier under-scoped
regex). Reusing or lightly adapting that already-hardened `parseOSC` logic
(or its `server/terminal/escape_scan.go` twin) carries materially lower risk
than writing a new implementation from scratch, LLM-generated or otherwise,
because the encoding edge cases (BEL vs. ST, partial reads, non-OSC
interleaving) are exactly the ones already found and fixed here twice.

**Verdict: Recommended — hand-roll, but by extending/reusing the existing
`pkg/analytics` (or `server/terminal`) OSC scanner rather than writing a
fresh one.** Writing a *fourth* independent implementation (this time inside
`session/detection`) would repeat the BUG-025 pattern (`pkg/ansi/csi.go`'s
docstring is literally about this: "the same ... bug was independently
reimplemented across the codebase ... before being fixed"). The lowest-risk
path is a small `pkg/ansi` addition (parallel to `CSIFinalByteClass`/
`StripCSI`) that extracts OSC payload content by number (e.g. `ExtractOSC(s
string, oscNum string) (payload string, ok bool)`), built on the same
BEL/ST termination rule already proven in `escape_code_parser.go` and
`escape_scan.go`, with `session/detection/detector.go`'s `ansiStripRegex`
continuing to strip the raw sequence from text as it does today (or,
better, being fed by the same pass).

## 4. Fork or adapt herdr's algorithm (not code)

herdr (Rust) treats OSC title as a higher-priority, debounce-bypassing
signal ahead of text-pattern detection (per the requirements' summary of
`src/detect/manifests/claude.toml` / `src/pane/agent_detection.rs`). The
*algorithmic* pattern worth porting (conceptually, not as code, since Rust
and Go share no source compatibility) is:

1. Parse OSC title/progress as a distinct, always-available signal alongside
   (not instead of) text-pattern matching.
2. Give the OSC-derived status precedence when it conflicts with text-derived
   status.
3. Let OSC-derived transitions bypass the debounce window that smooths
   noisy text-pattern flicker.

This is a priority/architecture decision (how the detector's signal sources
are ranked and debounced), not a parsing-library decision — it doesn't
change the build-vs-buy analysis above. It confirms the two signal sources
(OSC vs. text) should be threaded into the Claude detector as distinct
inputs (per requirement #2), with the OSC parser being the small extraction
function described in §3, not an imported crate/library equivalent.

## Final Recommendation

**Hand-roll**, by extending the existing hardened OSC-scanning logic already
in this repo (`pkg/analytics/escape_code_parser.go`'s `parseOSC` /
`server/terminal/escape_scan.go`'s `scanUntilTerminator`) rather than writing
a new implementation or adopting an external library:

- No external library is warranted — the ecosystem's options are either
  absent (no narrowly-scoped OSC-only Go package exists) or overkill (full
  VT100/VT220 emulators pull in a screen-buffer model this feature doesn't
  need), and none is already a direct dependency.
- Do **not** write OSC parsing from scratch a third time inside
  `session/detection`. This repo has already paid the cost of getting BEL/ST
  termination, partial-read handling, and ECMA-48 byte ranges right twice
  (and wrong once, per BUG-025, before `pkg/ansi` centralized the fix) —
  reuse that logic. The cleanest reuse point is a small addition to
  `pkg/ansi` (the package that already exists specifically to be "the single
  source of truth" for this class of escape-sequence rule) exposing an
  OSC-payload extractor built on the same termination rule, which
  `session/detection/detector.go` and the Claude binary detector can both
  call.
- SaaS is not applicable; herdr's contribution here is architectural
  (signal priority + debounce bypass), not a parsing implementation to port.
