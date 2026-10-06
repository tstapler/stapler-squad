# Build vs. Buy: Claude Code status-line footer detection

## 1. Existing OSS library / framework

**Parser for Claude Code's specific footer format:** none exists, and none is expected to.
The status-line/footer text (`"⏵⏵ auto mode on · N shells, M monitors · ← for agents"`,
`"✻ Cogitated for 1m 6s · done 3:06 PM · N shell, M monitor still running"`) is proprietary
Anthropic CLI UI, not a documented protocol, versioned schema, or published wire format —
confirmed bespoke-parsing territory. This matches the repo's own existing engineering
judgment: `session/detection/detector.go`'s `autoModeFooterRegex` comment and
`session/detection/binaries/claude.go`'s `WaitingForAgent` comment both explicitly track CLI
wording drift ("current CLI never emits ... using the persistent footer bar instead") as a
maintenance fact of life, not a spec violation.

**General terminal-output-parsing libraries (Go):** the repo already depends on what's
needed and uses it correctly — no gap to fill with a new dependency.
- `github.com/Azure/go-ansiterm` (`go.mod:106`, indirect) and `golang.org/x/term` (`go.mod:61`)
  are present for ANSI/PTY handling generally.
- `session/detection/normalizer.go`'s `PTYNormalizer.Normalize` (strip ANSI + collapse
  carriage-return overwrites) is the repo's own hand-rolled ANSI-aware pre-pass, run before
  any status regex sees the text. `session/tmux/banner_filter.go` similarly hand-rolls SGR/CSI
  stripping regexes for a different noise source (motd banners).
- A more "robust" general-purpose ANSI tokenizer (e.g. a full terminal-emulator library) would
  be over-buying: the repo doesn't need a virtual screen buffer, just linear text with escape
  codes removed, which `PTYNormalizer` already does. `session/detection/detector.go:206`'s
  comment about `esc\x1b[C to\x1b[C interrupt` cursor-forward encoding shows the one real
  ANSI-related wrinkle for status-bar text — it's already solved by the existing normalizer
  pipeline, not something a new library would additionally buy.

## 2. SaaS / managed API

Not applicable. This is local terminal scrollback parsing over text already captured from a
`tmux` pane — there is no network call, no hosted service, and nothing to outsource. (Same
conclusion the repo reaches elsewhere for OSC/statusline handling — it's a pure local
string-matching problem.)

## 3. LLM-generated bespoke regex vs. a tested library/tokenizer

A small hand-written regex with unit tests is the right level of investment, and the repo's
own precedent agrees:
- The parsing task is genuinely simple text-pattern extraction (two small integers plus
  optional "still running"/"still" qualifiers), not a algorithmically complex parse — no
  grammar, no nesting, no ambiguity requiring a real parser/tokenizer.
- The format is **inherently unstable** by the repo's own documented experience: `claude.go`'s
  comment block states plainly that three older `WaitingForAgent` patterns are "confirmed dead
  against Claude Code's current CLI output" — Anthropic changed the wording between versions,
  and the repo simply added a new pattern (`autoModeFooterRegex`) rather than versioning or
  generalizing a parser. This is direct evidence that over-engineering a general-purpose
  tokenizer for this text would still break on the next wording change, while a minimal,
  tolerant regex is exactly as fragile as the format itself — no more, no less — and cheap to
  patch when it inevitably drifts again.
- Existing test fixtures (`session/detection/testdata/*.txt`, `bug_regression_test.go`,
  `idle_test.go`) are all table-driven regex-input/expected-status cases — the established,
  low-ceremony test pattern to extend, not a reason to add tokenizer infrastructure.

**Recommendation:** minimal regex(es) + table-driven Go tests, following the exact shape of
`autoModeFooterRegex`/`footerAgentCount` and the `shells_still_running`/`monitors_still_running`
patterns already in the codebase. Keep it tolerant (e.g. don't over-anchor on exact glyph/
punctuation) precisely because the wording will change again.

## 4. Fork or adapt — closest existing analog

This is not a new detector category — it's an extension of an existing, almost-matching one.
Two existing pieces already cover most of the requested behavior:

- **`session/detection/detector.go`'s `autoModeFooterRegex` + `footerAgentCount` +
  `applyFooterIdleOverride`** (lines ~19–90) already parses exactly the *short* status-bar
  form from the requirements — `"⏵⏵ auto mode on · 1 shell, 1 monitor · ← for agents"` —
  via `auto mode on\s*·\s*(\d+)\s+shells?(?:,\s*(\d+)\s+monitors?)?`, sums shell+monitor
  counts, and overrides an idle/unknown verdict to "waiting" without ever masking a genuinely
  active turn. This is deliberately kept *outside* the per-line `PatternSet`/`StatusPatterns`
  match chain (see the block comment) because it's a persistent footer, not a turn-scoped
  status line — the same design constraint will apply to any new pattern added here.
- **`session/detection/binaries/claude.go`'s `WaitingForAgent` patterns**
  (`shells_still_running` / `monitors_still_running`, lines ~349–381) parse the mid-turn
  spinner-line form in isolation — `(\d+)\s+shells?\s+(?:still\s+)?running` and
  `(\d+)\s+monitors?\s+still\s+running` — but each matches only its own noun; neither handles
  the **comma-joined combined form** from the requirements' long example,
  `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"`. In that string,
  `shells_still_running` doesn't fire (the token after "1 shell" is a comma, not "running"),
  and only `monitors_still_running` fires, capturing the monitor count but silently dropping
  the shell count. These two patterns are also explicitly flagged as dead code for the current
  CLI (the block comment says so), which is corroborating evidence that they were written
  against an *older* wording and already drifted once — the exact risk called out in §3.
- On the UI side, **`web-app/src/components/sessions/SubStatusChip.tsx`**'s
  `SubStatus.WAITING_FOR_AGENT` case (lines ~38–60) already renders the requested "distinct
  visual indicator," including a count-aware tooltip ("Claude is waiting for N background
  task(s) to finish") whose comment explicitly names all three sources — background agents,
  shells still running, monitors still running — collapsed into one int by design ("task" is
  deliberately source-neutral). This chip requires no new UI component; AC2 is already met by
  existing code as long as the backend keeps emitting `SubStatus.WAITING_FOR_AGENT` with a
  count.

**Conclusion:** this backlog item is closer to "fix a gap in an existing detector" than "build
a new one." The concrete adaptation work is:
1. Add/adjust one regex (or extend `shells_still_running`) to correctly parse the comma-joined
   `"N shell, N monitor(s) still running"` form and sum both counts, following the exact style
   of the adjacent patterns.
2. Confirm the resulting count reaches `SubStatus.WAITING_FOR_AGENT` and the chip's
   `subagentCount` prop the same way the short auto-mode-footer form already does.
3. Add table-driven fixtures/tests for the new phrasing next to the existing
   `bug_regression_test.go`/`idle_test.go`/`claude_active_task_manager.txt`-style cases.

No new library, no new UI component, no new architecture — extend the two existing detectors
and their tests.
