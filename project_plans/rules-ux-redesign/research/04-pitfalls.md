# Research 04 — Pitfalls and Edge Cases

## 1. Edge Cases in CommandCriteria the UI Must Handle

### Programs: Prefix Matching Behavior

`matchesProgram()` in `command_parser.go` performs **prefix matching for versioned interpreters**:

```go
if strings.HasPrefix(prog, p+".") {
    return true // "python3" matches "python3.11", "python3.9", etc.
}
```

This means:
- A Programs entry of `"python3"` matches `python3`, `python3.9`, `python3.11`, `python3.12`, etc.
- A Programs entry of `"python"` matches `python`, but NOT `python3` (because `"python3"` does NOT start with `"python."` — the dot separator is required).
- A Programs entry of `"node"` matches `node` but NOT `node18` or `nodejs` (no dot separator in real node version names).

**UI implication**: When a user types `python3` in the Programs tag input, the helper text should say "Matches python3, python3.9, python3.11, and any python3.x variant." When they type `python`, note it only matches the unversioned `python` binary. Consider showing the matched set dynamically in the live preview.

### Subcommands: Prefix Matching for Deep-Subcommand Programs

Within `CommandCriteria.Matches()`:

```go
if sub == s || strings.HasPrefix(sub, s+" ") {
    found = true
}
```

For deep-subcommand programs (docker, gh, ip, aws, etc.), the extracted subcommand may be `"container logs my-container"` (two tokens captured by `extractSubcommand` + trailing positional). The prefix match means a rule entry of `"container logs"` matches `docker container logs my-container`. This is by design but non-obvious.

**UI implication**: The Programs field should display a "(deep subcommand)" indicator for `gh`, `docker`, `aws`, `kubectl`, `ip`, `gcloud`, `az`, `doctl`, `fly`, `heroku`. The Subcommands field help text should note: "For these programs, the subcommand matches prefix — 'container logs' also matches 'docker container logs my-container'."

### Subcommands: Single-Token Extraction for Non-Deep Programs

For regular programs (git, npm, cargo), only ONE subcommand token is captured, and only if it passes `isSubcommandLike()` (starts with letter, max 25 chars, only letters/digits/hyphens/underscores). This means:

- `git -C /repo status` → subcommand is `"status"` (flag + value skipped via `prefixFlagArgs`)
- `git reset HEAD~1` → subcommand is `"reset"` (HEAD~1 is not subcommand-like due to `~`)
- `npm run build` → subcommand is `"run"` (build is NOT captured as a second token for npm, which is not in `deepSubcommandPrograms`)

**UI implication**: Users building npm rules need to know that `npm run build` has subcommand `"run"`, not `"run build"`. The Programs field helper should note for npm: "Only the first subcommand token is captured (e.g., 'run' for 'npm run build')."

---

## 2. Conflicts Between Structured Criteria and Raw Regex

`classifier.matchesRule()` evaluates BOTH `CommandPattern` and `Criteria` with AND semantics:

```go
if rule.CommandPattern != nil {
    if !rule.CommandPattern.MatchString(cmd) { return false }
}
if rule.Criteria != nil {
    // ... evaluate criteria
    if !rule.Criteria.Matches(cmds[0]) { return false }
}
```

**If both `commandPattern` and structured criteria are set, the rule only fires when BOTH match.**

This creates subtle over-restriction:

- A user sets Programs: `["git"]` and also sets Command Pattern: `^git push` — the rule only fires for `git push` commands that also pass Programs matching. This doubles the restriction with no gain.
- A user migrates from a raw regex rule to a structured rule without clearing `commandPattern` — the old regex now silently narrows the new structured rule.

**UI implications**:
1. The two modes ("Structured" / "Regex") should be **mutually exclusive in the UI**. Switching to Structured mode should clear `commandPattern`; switching to Regex should clear all criteria fields. Prompt the user with a confirmation dialog before clearing.
2. If the backend receives a rule with both set (e.g., via API), the UI should display a warning banner: "This rule uses both a command pattern and structured criteria — both must match."
3. Do not expose `commandPattern` in Structured mode at all; hide it behind the "Regex" mode toggle.

---

## 3. Priority Ordering Pitfalls

### How Priority Works

Rules are sorted descending by `Priority` before evaluation. First matching rule wins. Seed rules use tiers:
- **1000**: AutoDeny (fires first)
- **500**: Escalate-before-allow (overrides allows at 100)
- **100**: AutoAllow (standard dev ops)
- **50**: Escalate catch-all

User rules default to `priority: 10` in the current form — **below all seed tiers**. This means:
- A user AutoAllow rule with priority 10 fires AFTER all seed rules. If a seed rule at priority 50 escalates the same command first, the user rule never fires.
- A user AutoDeny rule with priority 10 fires AFTER seed AutoAllow rules at 100 — so the deny never fires.

**This is the most critical UX pitfall.** A user who sets `priority: 10` on a custom AutoDeny rule will be confused when commands still get auto-allowed.

### UI Recommendations

1. **Show priority tiers visually**: The Priority input should show a tier guide: "Priority 1000+ = deny-first, 500+ = escalate-before-allow, 100–499 = allow, 1–99 = low priority (fires after built-in rules)."
2. **Smart priority defaults based on decision**:
   - AutoDeny → default to 1000 (user deny rules should fire first)
   - Escalate → default to 500 (user escalate rules should override allows)
   - AutoAllow → default to 100 (user allow rules at same tier as seed allows)
3. **Overlap warning**: In the live preview, if the current criteria would be matched by an existing higher-priority seed rule with a conflicting decision, show a warning: "Seed rule 'Block git push --force' (priority 1000, AutoDeny) will fire before this rule."
4. **Priority conflict detection**: On save, check the in-memory rule set client-side for overlap and warn.

---

## 4. Validation Requirements

### Regex Fields

Fields `commandPattern`, `toolPattern`, `filePattern` must be valid Go regex (RE2 syntax). The backend validates via `regexp.Compile()` in `RulesStore.Upsert()` and returns `connect.CodeInvalidArgument` on invalid regex.

**Frontend validation**: Use a client-side RE2-compatible regex validator. JavaScript's `RegExp` is mostly compatible but has differences (e.g. no `(?P<name>...)` named groups in Go → JS handles them differently). For safety, validate the regex on the backend on form submit; show the error inline. Consider a debounced "test pattern" call to the backend API as the user types in Regex mode.

**Invalid regex behavior**: If `regexp.Compile` fails, `specsToRules()` logs a warning and **skips the rule entirely** — the rule is saved to the DB but never evaluated by the classifier. This silent skip means a rule can appear in the UI as "Enabled" but never fire. The UI must make this clear: validate regex before saving.

### Structured Fields

No validation is required for Programs, Subcommands, or Flags beyond "non-empty string, no whitespace-only entries." However:
- **Programs should warn on unrecognized programs**: If the user types `pythn` (typo), the live preview will show no matches — but no error. Consider a fuzzy-match suggestion.
- **PythonModes**: Only valid values are `"inline"`, `"module"`, `"script"`, `"version"`. The UI should present these as checkboxes/toggles, not free-form text, eliminating invalid values.

---

## 5. Python-Mode Detection Edge Cases

### SafePythonImportsOnly Semantics

`SafePythonImportsOnly: true` in `CommandCriteria` only matches when ALL of the following are true:

1. The invocation IS inline (`-c` flag detected).
2. At least ONE `import` statement is present in the code.
3. ALL imported modules are in `safeStdlibModules` (see below).
4. NONE of the `bannedInlinePythonPatterns` appear in the raw command.

**Key gotchas**:
- **Bare code with no imports does NOT match**: `python3 -c "print('hello')"` has no import statements → `SafePythonImportsOnly` rule does NOT fire → falls through to the escalate catch-all at priority 50. Users may expect "print('hello')" to be allowed but it will escalate.
- **`sys` is on the safelist but `os` is not**: `python3 -c "import sys; print(sys.argv)"` is auto-allowed. `python3 -c "import os; print(os.getcwd())"` is NOT (os has system-call wrappers).
- **`io` is on the safelist but `io.open()` is banned**: The `bannedInlinePythonPatterns` list catches `io.open(` as a banned pattern even if `io` is a safeStdlibModule. The module-level safelist is necessary but not sufficient.
- **`open(` builtin is banned even without an import**: Code like `python3 -c "open('file').read()"` has no import but contains `open(` which is in `bannedInlinePythonPatterns` → rule does not match.

### safeStdlibModules Set (Complete List)

The safe modules are: `json`, `re`, `csv`, `xml`, `html`, `glob`, `pathlib`, `string`, `textwrap`, `pprint`, `struct`, `codecs`, `unicodedata`, `io`, `difflib`, `math`, `cmath`, `decimal`, `fractions`, `numbers`, `statistics`, `random`, `collections`, `heapq`, `bisect`, `array`, `queue`, `itertools`, `functools`, `operator`, `hashlib`, `hmac`, `base64`, `binascii`, `secrets`, `typing`, `types`, `abc`, `dataclasses`, `enum`, `copy`, `contextlib`, `ast`, `tokenize`, `keyword`, `token`, `datetime`, `time`, `calendar`, `sys`, `logging`, `warnings`, `traceback`, `argparse`, `shlex`, `uuid`, `locale`.

**Notably absent**: `os`, `subprocess`, `socket`, `pathlib.write_text` (method-level ban), `requests`, `httpx`, `urllib`, `threading`, `multiprocessing`, `pty`, `ctypes`, `importlib`, `inspect`.

**UI implication**: When `SafePythonImportsOnly` toggle is shown (only when `PythonModes` includes `"inline"`), display a tooltip or expandable panel showing the allowed modules list. The live preview should specifically show "Bare code (no imports) will NOT match this rule."

---

## 6. Deep Subcommand Programs and 2-Token Extraction

`deepSubcommandPrograms` (in command_parser.go) captures up to **2 positional subcommand tokens**:

```
gh pr create → sub = "pr create"
docker container run → sub = "container run"
ip route show → sub = "route show"
aws s3 cp → sub = "s3 cp"
```

For programs NOT in `deepSubcommandPrograms` (git, npm, cargo, go, etc.), only 1 token is captured:
```
git remote add origin → sub = "remote"  (not "remote add")
npm run build → sub = "run"  (not "run build")
```

**Critical pitfall for rule authors**: A user writing a rule to block `git remote add` cannot use Subcommands: `["remote add"]` because git only extracts 1 token. The correct approach is Subcommands: `["remote"]` (which also allows `git remote -v`, `git remote show`, etc. — less precise). The user would need a `commandPattern` regex like `^git remote add` for precise matching.

**UI implication**: The Programs field should show a "(single-level subcommand)" note for git, npm, cargo, go, etc. and offer a tooltip: "For git, only one subcommand word is matched. To match 'git remote add' precisely, use Regex mode with pattern `^git remote add`."

### Affected Programs (Deep)
`gh`, `aws`, `gcloud`, `az`, `doctl`, `fly`, `flyctl`, `kubectl`, `docker`, `heroku`, `ip`

### Affected Programs (Single — Partial List)
`git`, `npm`, `cargo`, `go`, `python`, `pip`, `uv`, `hugo`, `tailscale`, `systemctl`, `tmux`, `buf`, `pixi`

---

## 7. Seed Rules Interaction — Priority Conflict When User Adds Same-Priority Rule

### What Happens When Priorities Are Equal

`sort.Slice` with `rules[i].Priority > rules[j].Priority` is not stable in Go. When two rules have the same priority, their relative order is **non-deterministic** across restarts (sort order depends on the slice order, which is append order of seed + user rules in `allRuleSpecs()`).

Practically: seed rules are appended before user rules in `allRuleSpecs()`, so at equal priority, a seed rule tends to win. But this is an implementation detail, not a contract.

**UI implication**: Warn when the user saves a rule with a priority that collides with a seed rule. For example: if user sets priority=100 and there is a seed rule at priority=100 that overlaps the same program, show: "Another rule (seed-allow-bash-npm, priority 100) may match the same commands. If priorities are equal, evaluation order is not guaranteed."

### Seed-Rule Fields Not Exposed in RuleSpec

When seed rules are converted via `ruleToSpec()` in `rules_service.go`, the `Criteria` field is **not serialized** to the spec. The current mapping only extracts `ToolPattern`, `CommandPattern`, and `FilePattern` from the compiled rule — Criteria fields are lost. This means:

- Seed rules like `seed-allow-bash-python-run` (which uses `Criteria.Programs` and `Criteria.PythonModes`) appear in the UI with **empty CommandPattern** — they look like they match nothing based on existing UI.
- US-5 (readable rule display) cannot be implemented purely by reading existing proto fields from the server.

**Fix**: Extend `ruleToSpec()` to populate the new criteria fields from `rule.Criteria` (after the proto extension is done). Until the proto extension lands, a client-side lookup of seed rule details by ID is an alternative for rendering.

### User Cannot Override a Seed Deny Rule

The `RulesStore.Upsert()` enforces `Source: "user"`. A user cannot create a seed-priority rule via the API. If a user wants to override `seed-deny-git-reset-hard` (priority 1000, AutoDeny) with their own AutoAllow, they cannot — there is no mechanism to "shadow" or disable individual seed rules.

**UI implication**: Clearly label seed rules as "Built-in (read-only)" and explain: "Built-in deny rules run at priority 1000 and cannot be overridden. Contact your admin to modify them." This prevents user confusion when a custom allow rule doesn't fire as expected.
