# Findings: Features (UX Patterns — NL to Structured Form)

## Summary
Natural-language-to-structured-action is a well-established pattern in developer tools. The consensus pattern across Raycast, Linear AI, VS Code Copilot, and Cursor is: (1) one-shot parse with a spinner, (2) show a structured preview card, (3) allow inline editing before commit. The dominant trade-off is between speed (immediate creation) and safety (editable preview). For session creation — a consequential, hard-to-undo action — the editable form pre-fill approach used by Linear AI is the strongest model. Confidence handling is almost universally "best guess + editable" rather than asking for clarification, because clarification dialogs add friction that kills adoption.

## Options Surveyed

- **Raycast AI Extensions** — NL to command execution; shows preview of matched command + parameters before running
- **Linear AI** — "Create issue from description"; fills title, priority, assignee, labels from NL; shows editable form
- **VS Code / GitHub Copilot Chat** — `@workspace /new` and slash commands; NL → file scaffolding with preview diff
- **Cursor Composer** — NL to codebase edits; multi-file preview before applying
- **Notion AI** — NL to block content; inline generation, no form pre-fill pattern
- **Vercel v0** — NL to component code; shows output, user iterates
- **Command palettes (⌘K)** — Notion, Linear, Vercel: NL search + action matching, not structured extraction

## Trade-off Matrix

| Pattern | Ambiguity handling | Field coverage | Preview UX | Latency tolerance | Existing-vs-new |
|---------|-------------------|---------------|-----------|-------------------|-----------------|
| Immediate creation (Copilot /new) | Best guess, no review | Minimal (1-2 fields) | None — instant | <1s required | New only |
| Spinner + preview card (Raycast) | Show parsed, confirm | 3-5 fields | Compact card | 2-3s acceptable | Surfaces matches |
| Pre-fill editable form (Linear AI) | All fields editable | Full form (6-8 fields) | Full form review | 2-4s acceptable | Suggest existing inline |
| Iterative dialog (ChatGPT) | Ask follow-up Qs | Unlimited | Conversational | 10s+ | Can ask |

**Dominant trade-off**: Speed vs safety. Immediate creation is fastest but unrecoverable on error. Editable form is 2-4s slower but users correct mistakes before they become sessions. For a developer tool with complex parameters (path, branch, program), editable form wins.

## Risk and Failure Modes

**Editable form pre-fill (chosen approach)**:
- LLM picks wrong path → user sees it, corrects before submitting. Low risk.
- LLM takes >3s → spinner anxiety. Mitigate: show "Parsing intent..." immediately on `>` prefix detection; stream partial results if possible.
- User submits stale pre-fill while LLM is still running → debounce + disable submit button during parse.
- Low-confidence parse silently accepted → surface confidence score as a subtle indicator (e.g. amber highlight on uncertain fields).

**Existing session suggestion**:
- LLM suggests wrong existing session → user dismisses suggestion, creates new. Low risk.
- LLM misses a relevant existing session → acceptable; creation is the fallback.

**⌘K / prefix mode**:
- Users unaware of `>` prefix → discoverability is the main risk. Mitigate: tooltip on omnibar focus, `?` shows syntax help.

## Migration and Adoption Cost [TRAINING_ONLY — verify current stapler-squad omnibar impl]

- Existing omnibar is a search bar (React) — adding `>` prefix detection is a small change (~50 lines React)
- Existing `NewSessionModal` component already has all form fields — pre-filling it from an API response is well-defined
- New API endpoint `POST /api/sessions/intent` needed on Go side
- No breaking changes to existing session creation flow

## Operational Concerns

- **Latency perception**: 2-3s is acceptable if there is immediate visual feedback (spinner appears on `>` prefix, not on submit)
- **Streaming**: If the Anthropic SDK supports streaming JSON (partial tokens), progress can be shown field-by-field — substantially reduces perceived latency [TRAINING_ONLY — verify streaming JSON support]
- **Telemetry**: Track parse→form-open rate, form-edit rate (how often users change pre-filled fields), and form-submit rate to measure quality

## Prior Art and Lessons Learned

- **Linear AI issue creation** [TRAINING_ONLY — verify]: NL → title + description + priority + labels. Users accept pre-fills ~70% of the time without editing. Branch/assignee fields are harder to get right.
- **Raycast AI**: Command matching with NL is high-confidence; parameter extraction from descriptions is lower. They show a "confidence badge" and allow tab-to-edit.
- **GitHub Copilot /new**: Users complained about lack of preview — commits were made with wrong scaffolding. Linear's editable form approach avoids this.
- **VS Code Quick Input**: `vscode.window.showInputBox` with pre-filled value is the standard pattern for pre-populated prompts — user sees value and can edit inline before confirming.

## Open Questions

- [ ] Does Linear AI show field-level confidence indicators? — blocks decision on whether to highlight uncertain fields
- [ ] Can partial JSON be streamed from Claude API to show fields appearing one by one? — blocks decision on streaming architecture
- [ ] What edit rate do users exhibit on pre-filled forms in comparable tools? — informs how much we should optimize parse quality vs accepting edits

## Recommendation

**Recommended pattern**: Linear AI-style editable form pre-fill.

**Reasoning**: Session creation is a multi-field, consequential action. Users need to verify path and branch before committing. The editable form costs 2-4s of latency but gives full control. "Immediate creation" is off the table for this use case — a wrong path or branch would cause a confusing failure. The preview card is a reasonable middle ground but adds another interaction step vs just opening the form pre-filled.

**Specific UX decisions**:
1. `>` prefix in omnibar triggers LLM mode immediately (show spinner in omnibar)
2. On parse complete, open existing `NewSessionModal` with fields pre-filled
3. Highlight fields the LLM is uncertain about (confidence < 0.7) in amber
4. Show "Use existing session: [title]" banner at top of modal if `SuggestedSessionID` is set
5. Submit button disabled during parse to prevent race conditions

**Conditions that would change this recommendation**: If latency consistently exceeds 4s, switch to a two-step flow (preview card → open form on confirm) to avoid holding the omnibar open too long.

## Web Search Results

### 1. Linear AI — NL issue creation and form pre-fill

Web search did not surface explicit documentation of a "pre-fill form" pattern for Linear AI issue creation. What is confirmed: Linear's **Triage Intelligence** uses LLMs to infer issue properties (priority, assignee, labels) when issues are reported. Their **Product Intelligence** builds a semantic graph of issues for hybrid search and query rewriting. Linear positions itself as moving from "issue tracker to agent orchestration platform." The specific claim about ~70% acceptance of pre-fills is [TRAINING_ONLY — unverified].

**Conclusion**: The Linear AI approach (LLM → inferred fields → user confirms) is confirmed directionally, but the exact "editable form pre-fill" UX and edit rate data remain unverified. The recommendation stands on architectural reasoning even without the specific metric.

Sources: [linear.app/docs/creating-issues](https://linear.app/docs/creating-issues), [linear.app/now/design-for-the-ai-age](https://linear.app/now/design-for-the-ai-age)

---

### 2. Raycast AI — confidence indicator for NL command parsing

Web search did not confirm a "confidence badge" feature in Raycast AI. What is confirmed: Raycast's NL processing works by passing a JSON structure, predefined rules, and examples to an LLM to extract intent (date, time, priority for reminders). The AI Extensions API exposes `AI` utilities for natural language interaction. No confidence score surface in public docs.

**Conclusion**: The confidence badge claim is [TRAINING_ONLY — unverified]. The recommendation to surface amber highlights for low-confidence fields should stand on its own UX logic rather than citing Raycast as prior art.

Sources: [manual.raycast.com/ai](https://manual.raycast.com/ai), [developers.raycast.com/api-reference/ai](https://developers.raycast.com/api-reference/ai)

---

### 3. Claude API streaming JSON — partial response with tool use

**Confirmed**: Claude API streams tool input as `input_json_delta` events, each carrying a `partial_json` string fragment. Consumers accumulate fragments and parse once `content_block_stop` fires. The SDKs provide helpers for incremental/partial value access. Fine-grained tool streaming docs cover this explicitly.

**Implication for field-by-field pre-fill**: Yes, it is technically possible to show fields populating one-by-one as the JSON streams. Implementation: consume `input_json_delta`, attempt partial JSON parse on each delta (or use a streaming JSON parser), push field updates to the React form as they arrive. This requires a streaming JSON library (e.g., `github.com/buger/jsonparser` or custom accumulator).

Sources: [platform.claude.com/docs/streaming](https://platform.claude.com/docs/en/build-with-claude/streaming), [platform.claude.com/docs/fine-grained-tool-streaming](https://platform.claude.com/docs/en/agents-and-tools/tool-use/fine-grained-tool-streaming)

---

### 4. VS Code `showInputBox` pre-filled value pattern

**Confirmed**: VS Code's `window.showInputBox()` accepts a `value` parameter (pre-filled text) and `valueSelection: [start, end]` to control which portion is selected. `valueSelection` when undefined selects all; when `[n, n]` (empty range) places cursor only. This is the canonical "editable pre-fill" pattern for VS Code extensions and directly validates the recommendation to open `NewSessionModal` with fields pre-filled for user review.

Sources: [code.visualstudio.com/api/ux-guidelines/quick-picks](https://code.visualstudio.com/api/ux-guidelines/quick-picks), [microsoft/vscode-extension-samples quickinput](https://github.com/microsoft/vscode-extension-samples/blob/main/quickinput-sample/src/basicInput.ts)

---

## Pending Web Searches

1. `Linear AI natural language issue creation UX pre-fill form` — verify field coverage and edit rate data
2. `Raycast AI confidence indicator NL parsing 2024` — verify confidence badge pattern
3. `Claude API streaming JSON partial response tool use` — verify streaming JSON support for field-by-field pre-fill
4. `VS Code quick input pre-filled value pattern UX` — verify standard pre-filled input patterns
