# Requirements: Antigravity CLI + Open Code Feature Parity

## Context

Stapler Squad already has production-quality support for Claude Code across three feature areas:
1. **ssq-hooks**: Native PreToolUse hook registration via `installClaude()` / `patchClaudeSettings()`
2. **One-shot AI client**: `cli_ai_client.go` `knownCLIAgents` — Claude is `claude --print`
3. **Session detection**: `ClaudeDetector` with rich patterns for all status states (Ready, Processing, NeedsApproval, InputRequired, Error, Idle, Executing, Success)

Partial support exists for Antigravity CLI (agy v1.0.13) and Open Code (opencode). Gemini CLI already has parity in ssq-hooks (BeforeTool hook) but is out of scope.

## Known Gaps (Pre-Research)

### Antigravity CLI (agy)
| Feature | Claude | Agy | Gap |
|---------|--------|-----|-----|
| ssq-hooks install | `patchClaudeSettings()` ✓ | `patchAntigravityHooks()` patches BOTH config paths unconditionally | Logic should find first-existing like Gemini install does |
| One-shot AI client | `claude --print` in `knownCLIAgents` ✓ | **MISSING** | Add `agy --print` to `knownCLIAgents` |
| Detection patterns | Full (10+ patterns, all state types) ✓ | Mirrors Gemini (3 patterns: Ready, Processing, NeedsApproval) | Research real agy UI to add agy-specific patterns |
| Hook output format | `writeHookDecision()` JSON ✓ | `writeAntigravityHookDecision()` JSON ✓ | No gap |

### Open Code (opencode)
| Feature | Claude | OpenCode | Gap |
|---------|--------|----------|-----|
| ssq-hooks install | Native hook in settings.json ✓ | Proxy shell wrapper script ← | Research if opencode has native hooks; use if available |
| One-shot AI client | `claude --print` ✓ | `opencode run` in `knownCLIAgents` ✓ | Verify `run` args are correct |
| Detection patterns | Full ✓ | Processing + NeedsApproval + InputRequired only | Add Ready, Error, Idle, Success patterns |
| Hook output format | N/A | N/A (proxy-based) | N/A until hooks researched |

## Requirements

### R1: Agy one-shot AI client
Add `agy` to `knownCLIAgents` in `server/services/cli_ai_client.go` with `--print` flag (confirmed from `agy --help`). Priority: below Gemini, above OpenCode.

### R2: Agy hook install cleanup
Fix `installAgy()` to use first-found path logic (like Gemini's `installGemini()`) rather than unconditionally patching both `~/.gemini/config/hooks.json` and `~/.gemini/antigravity-cli/hooks.json`. Research authoritative path.

### R3: Agy detection patterns — real UI coverage
Research actual agy v1.0.13 TUI output strings. Add patterns for states currently missing: InputRequired, Error, Idle, Success. Update `AgyDetector.Patterns()` in `session/detection/binaries/agy.go`.

### R4: Open Code native hooks
Research whether opencode has a native hooks system (hooks.json or settings.json). If yes, implement `patchOpenCodeHooks()` and `writeOpenCodeHookDecision()` and replace the proxy wrapper with native hook registration. If no, document why proxy is the right approach.

### R5: Open Code detection patterns
Add missing detection pattern categories to `OpencodeDetector`: Ready (idle input prompt), Error, Idle, Success. Research opencode TUI output strings from opencode.ai docs and GitHub.

### R6: Open Code one-shot client validation
Verify `opencode run` is the correct invocation for one-shot non-interactive mode. Update args if needed.

### R7: Test coverage parity
Each new pattern or behavior must have a corresponding test. Test counts should be ≥ claude's test count for the same category.

## Out of Scope
- Gemini CLI (already has parity in hooks layer)
- Aider (different model entirely, not a gap)
- UI changes beyond what's needed for new programs (programs.ts already has both)
- New session creation modes

## Success Criteria
1. `ssq-hooks install agy` works correctly with single-path logic
2. `agy` appears in `NewBestAvailableAIClient` selection flow
3. `agy` sessions show correct status (Ready/Processing/NeedsApproval/etc.)
4. `opencode` sessions show correct status for all state types
5. `ssq-hooks install open-code` uses native hooks if available; proxy if not
6. All new behavior covered by tests
7. `make quick-check` passes
