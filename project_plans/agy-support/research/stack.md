# Research: Stack — Full agy Support

**Status**: Completed | **Phase**: 2 — Research  
**Created**: 2026-05-25

---

## 1. Language & Dependencies

- **Go** — All new code stays in Go, consistent with `cmd/ssq-hooks/main.go`. No new dependencies required.
- Existing standard library packages already in use: `encoding/json`, `os`, `os/exec`, `path/filepath`, `flag`, `fmt`, `strings`, `io`
- `github.com/google/uuid` — already imported in `cmd/ssq-hooks/main.go` for analytics
- `github.com/tstapler/stapler-squad/pkg/classifier` — `PermissionRequestPayload`, `ClassificationResult`, `RuleBasedClassifier` all available

## 2. JSON Patching Pattern (existing)

`patchClaudeSettings()` in `cmd/ssq-hooks/main.go` provides the canonical reference for idempotent JSON patching:
1. `os.ReadFile()` → create minimal `{}` if absent
2. `json.Unmarshal` → `map[string]interface{}`
3. Navigate/create nested keys: `settings["hooks"].(map[string]interface{})`
4. Idempotency check: scan existing entries for matching command string
5. Write to temp file, `os.Rename()` atomically (lines 459–553)

The same pattern works for Gemini/agy settings because both use flat JSON objects (not Claude's deeply-nested hook array format).

## 3. Gemini/agy Settings Schema

### agy: `~/.gemini/antigravity-cli/settings.json`
Current live file has no `hooks` key. Schema uses top-level keys: `allowNonWorkspaceAccess`, `enableTelemetry`, `permissions.allow[]`, `trustedWorkspaces[]`.

Hook injection adds:
```json
{
  "hooks": {
    "BeforeTool": "printf '%s' \"$TOOL_INPUT\" | ssq-hooks check --gemini"
  }
}
```

### Gemini CLI: `~/.gemini/settings.json`
Current live file uses a deeply-nested schema (`security`, `general`, `tools`, `experimental`, `ui`…) but no `hooks` key is present yet. Hook injection adds the same top-level `hooks.BeforeTool` string.

The existing `install-gemini-hook.sh` bash script confirms this shape: `.hooks.BeforeTool = "..."` (flat `jq` path).

## 4. BeforeTool Hook Format

Gemini/agy `BeforeTool` is a **shell string** (not Claude's JSON array `[{type, command}]` format). The shell string receives `$TOOL_INPUT` as an environment variable containing the tool payload JSON. The hook's stdout is interpreted by the CLI (exact semantics TBD — see pitfalls.md).

Confirmed from `install-gemini-hook.sh` line 31:
```bash
HOOK_CMD='echo "$TOOL_INPUT" | ssq-hooks check --db ~/.config/stapler-squad/stapler-squad.db'
```

Updated form for `--gemini` flag:
```bash
printf '%s' "$TOOL_INPUT" | ssq-hooks check --gemini
```

`printf '%s'` is preferred over `echo` to avoid issues with payloads containing backslash sequences.

## 5. Config File Search Order

For `ssq-hooks install gemini`, the config file discovery follows the same priority as the existing bash script plus the current live observation:
1. `~/.gemini/settings.json` ← exists in live env (use this first)
2. `~/.gemini/config.json` ← fallback
3. `.gemini.json` (project-local) ← lowest priority

For `ssq-hooks install agy`:
- Fixed path: `~/.gemini/antigravity-cli/settings.json`
- Create dir `~/.gemini/antigravity-cli/` if absent (`os.MkdirAll`)

## 6. Existing Partial agy Support (Branch State)

Already present in this branch:
- `config/config.go` line 680: `candidates` slice includes `"agy"`
- `pkg/classifier/command_parser.go` lines 428–431: `agy` entry in `recursiveEvalPrograms` with `passthroughSubcmds: {"proxy": true}` (same as `rtk`)
- `web-app/src/lib/constants/programs.ts` line 15: `{ value: "agy", label: "Antigravity", description: "Antigravity CLI (agy)" }`
- `web-app/src/components/sessions/SessionRow.tsx` lines 80–81: `◆` emoji for `gemini` and `agy`/`antigravity`

## 7. agy Binary Location

On the development machine: `/home/tstapler/.local/bin/agy` (confirmed via `which agy`).  
The `installClaude()` pattern copies to `~/.local/bin/ssq-hooks` — this is the same destination regardless of target.

---

## Summary

- **Stack**: Pure Go, no new deps; existing `encoding/json` + `os` suffice for JSON patching
- **Gemini/agy hook format**: single shell string under `settings["hooks"]["BeforeTool"]`; `$TOOL_INPUT` env var carries the payload
- **Reference impl**: `patchClaudeSettings()` + `copyBinary()` in `cmd/ssq-hooks/main.go` cover 80% of REQ-1/REQ-2 work
