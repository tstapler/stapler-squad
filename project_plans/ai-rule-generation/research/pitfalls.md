# AI Rule Generation — Failure Modes & Pitfalls

Researched against the live codebase. All file references are relative to the repo root.

---

## 1. Regex Safety

### Validation path (what exists)

`server/services/rules_store.go:87–93` — `Upsert()` calls `regexp.Compile(pat)` on all three pattern fields (`ToolPattern`, `CommandPattern`, `FilePattern`) before persisting. Invalid patterns are rejected with an error that propagates to the RPC caller as `CodeInvalidArgument`.

`specsToRules()` (same file, line 256–270) has a second compile step at load time; rules with invalid regex are silently skipped (a `Warn` log is emitted) rather than causing startup failure. This means a rule that passes validation at write-time could theoretically fail at load-time if the Go `regexp` version changes, though this is unlikely in practice.

### Gaps for AI-generated rules

1. **No ReDoS protection.** `regexp.Compile` confirms the pattern is syntactically valid Go RE2 syntax, but it does not bound matching time. An AI-generated pattern like `(a+)+$` or `.*.*.*foo` is syntactically legal and will compile successfully, but will cause catastrophic backtracking on long command strings that are near-matches. Every classifier evaluation holds the `RWMutex` in read mode during matching, so a slow regex stalls all concurrent classifications.

   Go's `regexp` package uses RE2 semantics (no backtracking exponential blowup), so true catastrophic backtracking cannot occur. **However**, RE2 can still take O(n²) time on certain patterns (nested alternation, wide character classes over long inputs). The risk is low with RE2 but non-zero for adversarially crafted inputs.

2. **Truncated preview used for reclassification.** `ReclassifyGaps()` (`analytics_store.go:271–278`) re-classifies coverage-gap entries using the stored `CommandPreview` field, which is truncated to 200 bytes at record time. If the AI proposes a `CommandPattern` that only matches the tail of a long command, `ReclassifyGaps` will misreport the gap as still open (false coverage gap), inflating the agent's confidence that the suggestion is needed.

3. **Pattern validation fires on `Upsert`, not on `GenerateSuggestedRule`.** The requirements (FR-1/NFR) say the backend must validate patterns before returning a suggestion. There is no existing validation hook in the suggestion RPC path — this must be added explicitly. Returning an invalid pattern to the UI without server-side validation would let the user accept a rule that is silently dropped by `specsToRules` on next load.

---

## 2. Rule Conflicts

### Priority model

Rules are sorted by `Priority` (descending) and evaluated first-match-wins (`classifySingle`, `classifier.go:506–528`). The sort uses `sort.Slice` (unstable), so rules with equal priority have undefined evaluation order. This is deterministic within a single process run but can change across restarts or whenever `AddRules`/`ReplaceRules` is called.

### User rules vs seed rules

`rebuildClassifier()` (`rules_service.go:197–207`) rebuilds the rule list by keeping non-user rules (seed + claude-settings) and appending the fresh user rules slice. User rules are appended **after** non-user rules in the slice. After `sort.Slice`, a user rule with the same priority as a seed rule has undefined precedence (unstable sort).

**Concrete conflict scenario:** A user accepts an AI-suggested `auto_allow` rule for `git push origin main` (a common "safe for this project" case) with default priority. If the AI suggests `priority: 100` (matching the seed allow tier) but the existing `seed-escalate-git-push` rule sits at `priority: 500`, the escalate rule will correctly win. However, if the AI suggests `priority: 600` to "override escalations," it will shadow the escalate rule as intended — but also silently shadow any other seed escalation rule at that priority tier. There is no conflict-detection pass that warns the user when a proposed rule will shadow an existing one.

**Second conflict scenario:** Two AI-generated rules with the same priority covering overlapping patterns have indeterminate evaluation order. The user sees a list of rules and has no indication which one fires first.

### No conflict detection at suggestion time

The `GenerateSuggestedRule` RPC (to be built) will have access to `allRuleSpecs()` as context (per FR-7). But the requirements do not specify that the backend should compute which existing rules the suggested rule would shadow or be shadowed by. Without explicit conflict analysis, the agent may suggest a rule that is silently dead (overridden by a higher-priority seed rule before it is ever evaluated).

---

## 3. Pattern Overfitting / Overbroad Allow Rules

### Existing examples of narrow vs. broad patterns

Seed rules demonstrate the risk explicitly. Compare:

- `seed-allow-gh-api-rest-jq` (line 965): `\bgh\s+api\b.*\s--jq\b` — safe because the 515-priority guard already blocked any `-f/-F/--field` flag before this fires. The 515-then-510 layering is intentional and documented.
- `seed-escalate-gh-api-explicit-write` (line 947): `\bgh\s+api\b.*(\s-X\s+(POST|PUT|DELETE|PATCH)\b|...)` — catches write-method combinations.

If the AI observes many `gh api ... --jq ...` escalations (because the seed rule is at priority 510 and a user's instance has a custom rule shadowing it), it might propose a broader `gh api` allow pattern without the priority layering awareness. The result would be an allow rule that fires before the 515 write-method guard, permitting `gh api --jq -f title=x`.

### Broader risk: the AI sees truncated previews

The analytics context passed to the agent (FR-7: `GetAnalyticsSummary`) uses `CommandPreview` — the **first 200 bytes** of the actual command. A command like:

```
curl -o /dev/null https://extremely-long-url.example.com/download?token=ghp_xxxx...
```

will have its tail (including the secret token) truncated. The AI sees only `curl -o /dev/null https://extremely-long-url...` and may propose `auto_allow` for curl with `-o` (downloading to a file), conflicting with the existing `seed-escalate-curl-download` at priority 500 (which catches `-o`/`--output`). The agent cannot see the full command.

### Concrete overfitting example from seed rules

`seed-allow-bash-git-write` (not shown in excerpt, but referenced at lines 1146, 1296) allows git write operations. An AI that sees many `git checkout` escalations might propose `commandPattern: \bgit\s+checkout\b` at priority 600. This would also auto-allow `git checkout -- .` (discards all working tree changes) and `git checkout -b my-branch origin/main -- .`, which are irreversible data-loss operations. The seed rule that handles these safely uses `Criteria`-based matching with `BlockedSubcommands`, which the AI cannot express via the string `commandPattern` field alone.

---

## 4. Latency Handling

### Current RPC timeout model

The existing long-running RPC is `RunOneShot` (`session_service.go:2701–2715`), which uses `context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)` with a 120 s default / 300 s maximum. The HTTP handler blocks until the subprocess returns or the context deadline fires.

### Risks for `GenerateSuggestedRule`

The requirements state the agent may take 5–30 seconds and the UI must show a meaningful loading state. ConnectRPC supports server-streaming and client-cancellation via context propagation. The key risks are:

1. **No streaming progress.** If `GenerateSuggestedRule` is a unary RPC, the client gets nothing until the response arrives. A 30-second wait with a spinner is acceptable per NFR, but if the AI provider's API hangs (e.g., network issue, rate limit retry loop), the goroutine leaks until the HTTP server shuts down unless a server-side deadline is applied independent of the client context.

2. **Client disconnect does not cancel AI call.** `approval_handler.go:311–316` shows the pattern: `case <-r.Context().Done()` catches client disconnect. The `GenerateSuggestedRule` handler must propagate `ctx` cancellation to the outbound AI provider HTTP request, or an aborted client request leaves an in-flight LLM call consuming tokens and goroutine resources.

3. **No cancellation RPC.** The requirements mention "allow cancellation" in the UI. ConnectRPC unary RPCs can be cancelled by the client closing the connection, but there is no dedicated cancellation mechanism. If the UI navigates away, the client HTTP connection closes, and the server goroutine must respect `ctx.Done()`. This works correctly only if the AI provider client is built with context propagation.

---

## 5. Privacy: Analytics Data and Sensitive Content

### What is stored

`AnalyticsData.CommandPreview` stores the first 200 bytes of the raw command string (`analytics_store.go:162–167`). There is **no redaction** of sensitive content in the preview — only truncation by length.

### The secret-scan ordering gap

In `approval_handler.go:150–168`, when `ScanForSecrets` detects a secret (e.g., a GitHub token in a curl command), the handler:
1. Calls `analyticsStore.RecordFromResult(payload, ...)` with the original `payload` — which contains the full command including the secret.
2. The `RecordFromResult` function then stores the **first 200 bytes of that command** as `CommandPreview`.

If the secret (e.g., `ghp_xxx...` at 40 chars) appears within the first 200 bytes — which it typically does since secrets are often passed as flags early in the command — the secret is persisted to the analytics SQLite database.

The analytics data is then surfaced to the AI agent (FR-7: `GetAnalyticsSummary`) as part of context assembly, meaning **the plaintext secret could be included in the prompt sent to the AI provider**.

### Recommended fix

The `RecordFromResult` call inside the secret-scan branch should use a sanitized payload — either clearing the `command` field entirely or replacing it with a placeholder like `[REDACTED: secret detected]` — before recording. The secret scan fires before analytics recording, so the fix is straightforward.

### Analytics table growth

`LoadWindow()` (`analytics_store.go:219–221`) loads **all rows** from the analytics table into memory (`ListAnalytics(ctx, 0)` with limit=0) and then filters by timestamp in Go. A `// TODO` comment acknowledges this. For `GenerateSuggestedRule`, the agent context assembly (FR-7) will call a variant of `GetAnalyticsSummary`, which uses this same code path. As the analytics table grows (months of usage, thousands of classifications per day), this in-memory load will become a latency and memory spike on every suggestion request.

---

## 6. Classifier Test Patterns Revealing Subtle Bugs

### Priority-shadowing (implicit in TestClassify_AddRules_HighPriorityFirst)

`classifier_test.go:247–270`: The test confirms a user rule at priority 9999 shadows the seed allow at 100. The test does **not** verify what happens when a user rule is added at priority 100 (the standard seed allow tier). In that case, `sort.Slice` (unstable) gives the user rule or the seed rule indeterminate precedence. The bug is latent: a user rule at exactly priority 100 may or may not win against a seed rule at the same priority depending on slice ordering from the previous sort.

### ReclassifyGaps uses truncated preview as full command (analytics_store.go:271–278)

`ReclassifyGaps` reconstructs a `PermissionRequestPayload` with `command: e.CommandPreview` and `file_path: e.CommandPreview` — the same truncated string for both fields. This means:
- A file-path pattern rule will match against the command preview text, not an actual file path.
- A command that was truncated mid-flag (`curl -o /tmp/myfile` → stored as `curl -o /tmp`) will be reclassified differently from the original.
- The AI agent uses `ReclassifyGaps` output to measure coverage gaps; gap counts may be incorrect for commands longer than 200 bytes.

### Compound command subcommand-prefix matching (classifier.go:211–222)

`CommandCriteria.Matches` uses prefix matching for subcommands to handle container names after `docker logs <name>`. The comment says `"logs my-container" matches rule entry "logs"`. A naive AI-generated `Criteria.Subcommands: ["rm"]` rule would match not just `docker rm <container>` but also any subcommand that starts with `rm`, including a hypothetical `docker rmfoo` or a future CLI subcommand. This prefix matching is intentional in the seed rules but is not obvious from the proto field names. An AI generating `Criteria` JSON might not be aware of the prefix semantics.

---

## Summary

| # | Risk | Severity | Code Location |
|---|------|----------|---------------|
| 5a | **Secret persisted to analytics then sent to AI provider** | Critical | `approval_handler.go:156–163`, `analytics_store.go:162–167` |
| 2 | **No conflict/shadowing detection for suggested rules** | High | `rules_service.go:196–207`, `classifier.go:506–527` |
| 3 | **Overbroad allow pattern bypasses layered escalation guards** | High | `classifier.go:877–975` (priority tier design) |
| 4b | **Client disconnect does not cancel outbound AI call** | Medium | `approval_handler.go:311–316` (pattern to follow) |
| 1a | **AI pattern validated for syntax but not for match breadth** | Medium | `rules_store.go:87–93` |
| 5b | **LoadWindow full-table scan degrades suggestion latency** | Medium | `analytics_store.go:219–221` |
| 1b | **Pattern validated at write but silently skipped at load** | Low | `rules_store.go:255–270` |
| 6b | **ReclassifyGaps uses truncated preview; coverage counts may be wrong** | Low | `analytics_store.go:263–285` |
