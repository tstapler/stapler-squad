# ADR-008: Secret-entry limitation and fixture privacy

## Status
Accepted (2026-11-08). Owner-visible risk: call out in the PR-A description.

## Context
ADR-002 keeps Gboard autocorrect and the suggestion strip ON. The layer cannot know whether the terminal is at an echo-off prompt (sudo, ssh, tokens), so a secret typed there passes through a normal textarea: Gboard may show it in the suggestion strip and learn it into the personal dictionary. Separately, device event recordings contain whatever the owner typed.

## Decision
1. The layer sets `autocomplete="off"` and `autocapitalize="none"`. It does not set `autocorrect="off"` or `spellcheck="false"` in v1 because no reliable echo-off signal exists (cursor-hidden heuristics are not reliable; Task 1.2.1c records the check). `attributesFor({echoOff})` exists as data so a future signal can switch them on.
2. The limitation is named in `docs/reference/mobile-input-layer.md`, the how-to, and the PR-A description. The workaround for secrets is a hardware keyboard, a password manager's fill, or a terminal-side paste.
3. Diagnostics: the redacted report is content-free. Content-bearing capture is a separate opt-in, local-only mode that records one short user-initiated session typed into a scratch field, never auto-sends, clears on close, and requires the scrub checklist (Story 4.1.3) before any fixture is committed. A lint test rejects committed `device` fixtures without `scrubbed: true` and a `signoff`. This runs over every committed fixture in CI and is a soft guard (self-attested fields). "Reveal text" content lives only in an in-memory buffer in the sheet component, cleared on close, and never feeds the report. Copy and Download write captured content to the clipboard (Gboard clipboard history) and Downloads; the review UI and the device-fixture PR template say so.
4. Typed data never appears in `cm.*`, `console` or `onAnomaly` payloads (tested).

## Alternatives rejected
Disabling autocorrect globally (kills the feature owner asked for); a user-visible "secure input" toggle in v1 (extra surface, can be added later via `attributesFor`).

## Consequences
Residual risk remains until an echo-off signal exists. It is documented, not mitigated.

## Amendment 2026-11-08 (triad review): spoken echo
The TalkBack committed-word announcement (plan Story 1.4.4) is also a secret-entry exposure: a word typed at a `sudo`/`ssh`/token prompt is read aloud, in addition to the suggestion-strip and personal-dictionary exposure above. The layer still cannot detect echo-off prompts, so the limitation is documented (reference docs secret-entry section, Task 5.1.2a) rather than mitigated; the announcement is written to an in-memory live region, cleared after 1 s, and never logged.
