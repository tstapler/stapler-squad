# Validation: Built-in Changelog

## Adversarial Review

### Challenge 1: `//go:embed` path constraint

**Claim:** Embed `CHANGELOG.md` in `package main` at repo root.

**Verdict: VALID.** Go's `//go:embed` directive requires the embedded file to be in the same directory as the `.go` file or a subdirectory thereof. `CHANGELOG.md` is at the repo root; placing `changelog_embed.go` (or adding an embed directive to `main.go`) in `package main` satisfies this constraint. Passing the bytes to `pkg/changelog` via an assignment (e.g., `changelog.Content = changelogMD`) is idiomatic and avoids `../` relative paths.

**Watch out:** The file must not be `.gitignore`'d (it isn't) and must be present in any worktree. This is already satisfied since `CHANGELOG.md` is a tracked source file.

---

### Challenge 2: `norawghrequest` rule compliance

**Claim:** Use `newGHRequestForHostWithToken` / `NewConditionalRequest` for the GitHub releases API call.

**Verdict: VALID BUT REQUIRES CARE.** The `norawghrequest` analyzer (`tools/lint/norawghrequest`) flags any `http.NewRequest`/`http.NewRequestWithContext` whose URL resolves to `GhBaseURL()`-derived paths. The releases endpoint is `https://api.github.com/repos/{owner}/{repo}/releases/latest` — this is a GitHub REST call and falls squarely in scope. The implementation must use `NewConditionalRequest` (which requires an `*ETagCache` instance) or `NewConditionalRequestNoCache` for a simpler one-off call. The update check service should receive an `*ETagCache` injected at construction time.

**Action:** Before implementation, run `make lint-custom` locally after adding `github/releases.go` to confirm no analyzer violations.

---

### Challenge 3: ETag cache ownership for update check

**Claim:** Pass `*ETagCache` to `UpdateCheckService`.

**Concern:** `ETagCache` is currently used for PR polling. If the releases check shares the same cache instance, a collision on cache key is possible.

**Verdict: LOW RISK.** `ETagCache` keys are the full request URLs. `https://api.github.com/repos/tstapler/stapler-squad/releases/latest` will never collide with a PR URL. But to be safe, the update check service should use its own dedicated `ETagCache` instance (or `NewConditionalRequestNoCache` with its own in-memory ETag stored in the service struct).

---

### Challenge 4: Source build detection

**Claim:** Skip update check when `buildinfo.Branch != ""`.

**Concern:** A locally compiled binary on `main` branch might still want to know if there's a newer tagged release (the branch is `main` but the commit is behind `HEAD`).

**Verdict: PARTIALLY ADDRESSED.** The requirement says to show a "source build" indicator rather than an update nudge for builds with non-empty `Branch`/`Commit`. This is a deliberate design choice (source builds track a branch, not a release train). The plan correctly skips the network call for these builds. If the owner later wants "also check for releases even for source builds," that's a follow-up.

---

### Challenge 5: Parser robustness

**Claim:** A simple state-machine parser handles the CHANGELOG.md format.

**Concern:** The CHANGELOG.md is 2280 lines and covers many releases. Malformed entries or format drift could cause the parser to panic or produce garbage.

**Verdict: MITIGABLE.** The parser should:
1. Never panic — use `recover()` or defensive nil checks.
2. Return partial results on malformed input (not an error).
3. Be tested against the actual `CHANGELOG.md` as a golden-file test (parse must produce at least N releases where N matches a manually counted floor).
4. Be robust to extra blank lines, missing commit links, multi-line bullets (treat continuation lines as part of previous entry).

---

### Challenge 6: Update check makes outbound call without user consent

**Claim:** Default ON with an opt-out in settings.

**Concern:** This contacts `api.github.com` from the user's server (or home machine), leaking the user's IP to GitHub. Some users run stapler-squad on sensitive machines.

**Verdict: DESIGN DECISION NEEDED.** Two options:
- **Option A:** Default OFF, opt-in from Settings. Safer default; users who want update notifications turn it on.
- **Option B:** Default ON, prominent opt-out toggle in Settings, documented in README.

Recommendation: **Option A (default OFF)**. The changelog viewer (Epic 1 + 3) has full value without the update check; the check is a nice-to-have. This avoids any privacy concern and keeps the feature deliverable without a policy discussion.

---

### Challenge 7: Feature registry compliance

**Claim:** Run `make registry-generate` before the PR.

**Verdict: REQUIRED.** Per `CLAUDE.md`, running `make registry-generate` is mandatory when adding new RPCs or React pages. The `/api/changelog` handler and `/changelog` page both require this. Failure to do so causes the `registry-diff` CI check to fail.

---

## Revised Acceptance Criteria Delta

Based on the adversarial review, one AC should be clarified:

- **AC 5 (update check):** Change default from "ON" to **OFF** (opt-in). The update check is disabled by default; users can enable it from Settings. This eliminates the privacy concern as a blocker.

## Go/No-Go

**Go.** The feature is well-scoped, uses proven patterns from the codebase (`//go:embed`, `NewConditionalRequest`, feature flags, the settings/routes pattern), and the risks are all mitigable with standard techniques. The revised AC 5 (opt-in update check) removes the only design-level concern. No new external dependencies are required.
