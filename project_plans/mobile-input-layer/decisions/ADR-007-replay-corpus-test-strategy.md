# ADR-007: Replay corpus and test strategy

## Status
Accepted (2026-11-06)

## Context
jsdom has no IME; Playwright/CDP cannot reproduce Gboard (`research/pitfalls.md` P4). jscpd is a 0.14% ratchet dominated by `jest.mock` blocks; BUG-087 forbids "gate" in test filenames.

## Decision
- Correctness proof: one table-driven Jest test replays JSON traces from `web-app/src/lib/terminal/mobileInput/__fixtures__/*.json` (`{id, source: "synthetic"|"device", keyboard, events[], expectedBytes, finalText}`) through the pure reducer into a model terminal (applies DEL/text to a string). Synthetic and device counts are reported separately; a validator fails if a fixture lacks `source`.
- `fast-check` properties: permutation of permitted event orders (compositionend before/after beforeinput) yields identical bytes; model-terminal line equals final textarea value; no DEL exceeds `sent`; grapheme-safe counts.
- One parametrized integration test over both renderers (jscpd).
- Playwright mobile-emulated e2e proves wiring only (focus, flag, bytes reach pane, both renderers), using a new CDP `Input.imeSetComposition` helper; its header states it does not prove Gboard fidelity.
- Go tests for the metrics helper and handlers use `t.Parallel()`, injected loggers.
- Final gate: owner phone acceptance. Device fixtures come ONLY from the opt-in local-only 'capture for fixture' mode (Story 4.1.3), never from the content-free diagnostics report (which cannot be replayed). A `source:"device"` fixture must carry `scrubbed: true` and a non-empty `signoff`, passes the scrub checklist and a secret-pattern scan, and the PR carries owner sign-off; a lint test fails otherwise. Before such fixtures exist the report reads `device 0/0`.
- JSON fixtures are excluded in `web-app/.jscpd.json` (Task 0.1.4a) so near-identical traces do not count toward the 0.14% ratchet. A jest version-pin test fails on any ghostty-web version other than the verified one and the ghostty double-send e2e is a required check on dependency changes (Tasks 1.4.2b, 5.1.1d). The corpus includes `backspace-on-empty-after-commit` and `hardware-keyboard-keydown` fixtures (Task 1.1.4e).
- Integration test mocks the `ghostty-web` WASM module once in a shared helper and tests the layer against a fake renderer handle; real ghostty behavior is covered by Playwright only.

## Alternatives rejected
One test file per scenario (jscpd); trusting Playwright as IME proof.

## Consequences
The 100% replay pass figure must be quoted with its synthetic/device split.
