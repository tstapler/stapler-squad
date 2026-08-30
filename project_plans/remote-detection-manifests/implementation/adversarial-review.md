# Adversarial Review: remote-detection-manifests
**Date**: 2026-08-06
**Verdict**: BLOCKED → **RESOLVED 2026-08-06** (both blockers patched into `plan.md` during
`/sdd:4-validate`; see checkbox notes below)

Both blockers below are independently re-verified against the live repo (HEAD `3ef07de64`,
2026-08-06), not taken on trust from the plan's own citations. The plan's core factual claims
(PR #307 closed/unmerged, `3c25e94f9`/`32f504c8` not ancestors of `main`, no plugin-loader
symbols anywhere in the tree, the 12-file/2617-insertion scoped diff count) all reproduced
exactly. This is a well-researched plan with a genuinely sound two-phase gating structure; the
issues below are specific design/scope gaps, not a rejection of the overall approach.

## Blockers

- [x] **RESOLVED 2026-08-06** — `plan.md` Task 2.2.2a/2.2.2b now specify an explicit bootstrap
  branch (`cachedVersion == ""` short-circuits to accept, bypassing `compareManifestVersion`)
  and a corresponding test. **Phase 2's version-comparison has no defined bootstrap case, and as specified it would
  reject every fresh install's first-ever fetch.** `compareManifestVersion` (ADR-001, Task
  2.2.1a) hard-errors on a non-numeric-segment version string; `shouldAcceptManifest` (Task
  2.2.2a) calls it as `compareManifestVersion(fetchedVersion, cachedVersion)` with no
  documented behavior for the case where no cached manifest exists yet (`cachedVersion == ""`,
  the state of every install before its first successful fetch, per the Migration Plan's own
  "created empty on first run" note). `""` is not valid dotted-numeric input, so the very first
  fetch on a brand-new `detectors-remote-cache/` would hit the malformed-version error path and
  be rejected — permanently, until some other code path clears the error, which nothing in the
  plan specifies. None of Story 2.2.1's or 2.2.2's acceptance criteria test "no cached manifest
  exists." — Add an explicit bootstrap branch (e.g. `shouldAcceptManifest` short-circuits to
  accept when no cached manifest/version exists, bypassing `compareManifestVersion` entirely)
  and a corresponding acceptance criterion, before Task 2.2.2a is implemented.

- [x] **RESOLVED 2026-08-06** — `plan.md` Story 1.2's Files list and acceptance-criterion diff
  command now include `session/instance_terminal.go` and `session/claude_controller_test.go`.
  **Story 1.2's "Files" list and its own re-verification diff command both omit real,
  substantive files from the historical diff, undermining the "no unexplained deviation"
  acceptance criterion.** Verified directly: `git diff 32f504c80 c64d94cf8 --diff-filter=M
  --name-only` shows `session/instance_terminal.go` and `session/claude_controller_test.go` are
  both modified with real content — `instance_terminal.go` adds `Instance.GetProgram()` (13
  lines, a new accessor `session/claude_controller.go`'s `ResolveDetectorForProgram` call site
  depends on to resolve the running program's detector) — but neither file appears in Story
  1.2's "Files" list, and both are excluded by the acceptance criterion's own diff command
  (`git diff <rebase-base> HEAD -- session/detection main.go session/claude_controller.go`,
  which never mentions `session/instance_terminal.go`). A future implementer following the
  Files list literally would not know to recreate/verify `GetProgram()`, and the "matches the
  reviewed design with no unexplained deviation" check would silently pass even if that
  accessor were dropped or altered, because the check's own scope excludes the file that
  carries it. — Add `session/instance_terminal.go` and `session/claude_controller_test.go` to
  Story 1.2's Files list and widen the re-verification diff command to include them (or drop
  path-scoping entirely and diff the full historical range, which is only 17 files).

## Concerns

- [ ] **"Background refresh" is one-shot per process lifetime, not periodic — this weakens the
  feature's own value proposition for a long-lived service.** `refreshRemoteManifests` is
  launched exactly once, guarded by `initRemoteManifestsOnce sync.Once` inside
  `InitRemoteManifests` (Task 2.4.2c); no ticker/interval/re-fetch trigger exists anywhere in
  the plan (`grep -i "ticker|periodic|interval|cron"` over the plan file: zero matches).
  `stapler-squad` runs as a long-lived systemd user service (`.claude/rules/
  systemd-user-service.md`), so a manifest fix published while an instance is already running
  will never reach it until the next service restart — which is exactly the "full release"-style
  event Phase 2 exists to avoid. `requirements.md`'s own Success Metrics phrasing ("on next
  start (**or background refresh**)") implies periodic refresh is part of the bar; as designed,
  the plan only delivers the "next start" half. — Either add a periodic re-fetch task (interval
  configurable/reasonable default, e.g. hourly) to Epic 2.4, or explicitly narrow the Success
  Metrics/requirements wording to "next process start only" so the gap is a documented choice,
  not an accidental one.

- [ ] **The mis-closure's root-cause investigation (Story 1.1) re-runs the same checks but never
  establishes the actual mechanism, which is independently discoverable and strengthens the
  correction.** I found it while re-verifying: PR #307's cited base commit `32f504c803add...`
  ("chore(sdd): validation artifacts + review-driven patches for ci-hookurl-race-flake") and a
  commit on `main`, `7978c22938772...`, share the identical author, identical timestamp
  (`2026-08-01 12:23:26 -0700`), and identical commit message, but different SHAs — i.e., that
  specific chore(sdd) planning-artifact commit really did land on `main`, just via a different
  lineage/hash than the one on the closed PR's branch. This is almost certainly what the closer
  actually checked (matched on message/timestamp rather than verifying hash-ancestry) before
  wrongly generalizing that conclusion to the two unrelated *feature* commits (`36a951acb`/
  `c64d94cf8`) stacked on top of that base — which never landed anywhere. Task 1.1a's checks
  (already good) prove the closure is wrong but don't explain why it happened; without that,
  the correction paragraph (Task 1.1b) can only assert "verification contradicts it," not name
  the mechanism. — Add a task comparing `32f504c80`'s tree/content against its same-titled
  counterpart on `main` to make this concrete in the re-land PR's description.

- [ ] **`rebuildSnapshot`'s "three call sites" (flagged as approach (a)'s stated weakness) are
  never enumerated anywhere in the plan.** The Pattern Decisions table names this cost but no
  task in Epic 2.3/2.4 lists which call sites need the new two-argument signature. By inspection
  of the historical diff there are at least three even before Phase 2 (initial `InitPlugins`
  scan, fsnotify-triggered local rebuild) plus two more Phase 2 adds (`InitRemoteManifests`'s
  synchronous scan, `refreshRemoteManifests`'s post-fetch rebuild) — closer to four or five than
  three. — Enumerate the call sites explicitly in Task 2.3.1a so a signature change doesn't miss
  one.

- [ ] **Cache-directory-creation failure (e.g. permission denied, disk full) has no specified
  behavior or test.** `InitRemoteManifests`'s `os.MkdirAll(remoteCacheDir, 0o755)` (Task 2.4.1a)
  can fail, and the function returns `error` — but neither Story 2.4.1's acceptance criteria
  (which cover only network-unavailable and pre-populated-cache cases) nor any task states
  whether a `MkdirAll` failure should be fatal or logged-and-continue. The historical Phase 1
  precedent (`InitPlugins`'s call site in the actual `main.go` diff: `if err :=
  detection.InitPlugins(ctx); err != nil { log.Warn(...) }` — never fatal) strongly implies the
  same convention should apply here, and the plan's own "fetch failure must never block
  startup" NFR arguably extends to "cache-directory-creation failure must never block startup"
  too — but this is inferred, not stated or tested. — Add an acceptance criterion + test for the
  `MkdirAll` failure path, and state explicitly that `main.go`'s call site treats
  `InitRemoteManifests`'s error the same non-fatal way as `InitPlugins`'s.

## Minors

- Task 1.2.1/1.2.2's "expect conflicts limited to import ordering... adjacent unrelated changes"
  framing is accurate as of right now (verified: zero commits have touched
  `session/detection/registry.go`, `detector.go`, or `binary_detector.go` since the branch's
  original merge-base `14e26b7ba`, and only 2 commits total touch any of `session/detection/`,
  `main.go`, or `session/claude_controller.go` in that whole window) — but this is a claim with
  a shelf life in an actively-developed repo. Not action-blocking; just don't treat the low
  current conflict risk as guaranteed to still hold whenever Phase 1 actually executes.
- ADR-002 doesn't mention that unauthenticated `raw.githubusercontent.com` requests are subject
  to GitHub's shared-IP rate-limiting/abuse throttling, which is a plausible (if low-stakes,
  since it fails safe to cache/bundled) cause of "fetch always seems to fail from this network."
  Worth a one-line addition to the ADR's residual-risk section, not a design change.
- `requirements.md` Open Question #5 (whether the 90-day checkpoint itself is tracked anywhere
  durable) is honestly named in the plan's Unresolved Questions but has no owner or follow-up
  task even outside this plan's scope — acceptable given it's explicitly deferred to
  `detector-plugins`, just noting it has no current owner anywhere.
