# Adversarial review: go-git worktreeConfig plan

Reviewed: `implementation/plan.md` against `requirements.md`, `research/*.md`, backlog item 9c33e913 (11 ACs), the fork source at `~/go/pkg/mod/github.com/tstapler/go-git/v5@v5.19.3-ssq.1/repository_extensions.go`, and stapler-squad's open/vet paths.

**Verdict: BLOCKED** (1 blocker: the planned patch cannot satisfy AC3).

## How I checked
I copied the fork to the scratchpad and applied the plan's T1.1 literally: lowercase the three `extensionsValidForV0` keys and add `"worktreeconfig"` to `builtinExtensions`. I then ran a throwaway module (`replace` pointing at the patched copy). It built real repos with git 2.53 and opened them with `PlainOpenWithOptions{DetectDotGit, EnableDotGitCommonDir}`, the same options as `session/git/native/open.go:19`.

| Case on patched fork | Result |
|---|---|
| V0 + `extensions.worktreeConfig=true` | Opens. `Reference(refs/heads/main)` resolves to a SHA. |
| V1 + `extensions.worktreeConfig=true` | Opens. Reference resolves. |
| V0 + `extensions.partialClone` + `extensions.preciousObjects` | **Fails**: `unknown extension: partialclone, preciousobjects` |
| V0 + `extensions.foo` | Fails with `does not support extension: foo`, as intended. |
| V1 repo after go-git rewrites its config (`CreateBranch`) | Config gains `objectformat = `. Re-open fails with `unknown extension: objectformat`. |

The existing fork `TestVerifyExtensions` cases still pass with the patch.

## BLOCKER

**B1. The plan violates AC3 for `partialclone` and `preciousobjects`.**
- Backlog AC3: "`verifyExtensions` accepts `partialclone` and `preciousobjects` for V0 repos without error."
- `repository_extensions.go:90-104` is the V0 gate. Lowercasing the keys (lines 49-52) only gets these two past it.
- `repository_extensions.go:106-117` is the second gate. Both extensions are absent from `builtinExtensions` (lines 32-43), so the patch ends in `ErrUnknownExtension`. The probe confirmed this.
- Plan T1.1 adds only `worktreeconfig` to `builtinExtensions`, and `plan.md:6` maps this change to "AC1-3".
- Plan T1.2's test table lists "V0/V1 x worktreeconfig, partialclone, preciousobjects". If a case expects acceptance, it fails. If the case is written to match the behavior, AC3 is silently not met.
- `research/pitfalls.md:2` and `requirements.md:35` both say these should be "still accepted". Nothing in the repo shows they were ever accepted, so "still" is wrong.
- Fix, choose one and record it in the plan:
  - (a) Add `partialclone` and `preciousobjects` to `builtinExtensions`. This meets AC3 literally. Risk: `partialClone` means promisor objects, and go-git cannot lazy-fetch them, so reads can fail later with "object not found". Add a comment saying so.
  - (b) Get the owner to amend AC3 to "partialclone/preciousobjects are rejected with a clear error".
- I recommend (a) plus a comment, because AC3 is explicit. Either way, T1.2 needs explicit expected outcomes per extension and version.

## CONCERN

**C1. A parallel plan covers the same bug and proposes a different fix.**
- `project_plans/fix-worktreeconfig-extension-compat/` exists, untracked, with plan, pre-mortem, ADR-001 and its own adversarial-review.md.
- Its requirements add a "Bug 3": `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` should go through the backend instead of `OpenRepo` (`session/git/ops.go:87,104`).
- This plan does not address that. Decide which plan is authoritative before implementing, so two sessions don't land overlapping fork tags or `go.mod` bumps.
- Related memory entry: re-check parallel work at ship time.

**C2. The fix makes the ResolveX functions succeed, but the plan relies on the bug's own symptoms as proof.**
- The probe verified `PlainOpen` and `Reference()` on both format versions.
- For `ResolveOriginBranchSHA`, the fetch step at `ops.go:86` goes through `gitprovider.Backend()`, not go-git (`ops.go:28-45`).
- So the fetch behavior is independent of the fork. The issue log shows git-CLI fetch errors alongside the go-git open errors.
- T2.2 must wire the backend the way `ops_test.go:317-345` does, or the fetch fails and the test cannot reach `OpenRepo`.
- T2.2 should also assert that origin's tip, not a stale local ref, is returned. The existing `_ReturnsFetchedTip_When_OriginHasAdvanced` test (`ops_test.go:317`) is the pattern.
- The repo setup must use real `git config` to set both `core.repositoryformatversion=0` and `extensions.worktreeConfig=true`.

**C3. After the fix, go-git writing config to a V1 repo bricks it.**
- `config/config.go:474-480` (`marshalExtensions`) writes `extensions.objectformat = ` (empty) on every `SetConfig` when the format version is 1.
- Before the fix this was unreachable for V1+worktreeConfig repos, because open failed first.
- After the fix, any go-git config write (`CreateBranch`, `SetConfig`, `CreateRemote`) on such a repo:
  - makes the next go-git open fail with `unknown extension: objectformat`;
  - makes real git reject the repo. I verified `git status` fails with `error: invalid value for 'extensions.objectformat': ''`.
- Stapler-squad production code does not appear to call go-git config writers. The only hit is the test fixture `session/unfinished/internal/testrepo/fixtures.go:81`. Config writes go through the CLI backend (`session/git/backend/cli/refs.go:278`).
- The risk is latent but real, and it is a new exposure created by this fix. Mitigate with one of:
  - add a plan note and a grep-based guard;
  - in the fork, make `marshalExtensions` skip an empty `ObjectFormat`. This is a 1-line change, and the plan's fork patch could carry it.

**C4. Fork docs will be wrong after this change.**
- The fork's `README-SSQ.md` says "Zero patches today ... No Go source differs from upstream."
- `docs/how-to/roll-back-go-git-fork.md` ("Verified" section) says the tag "carries no source patches".
- After ssq.2, rolling back to stock `v5.19.3` also reintroduces this exact bug. The rollback doc needs a one-line caveat.
- The plan has no task for any of this. Add one to T1.3 or T2.1.

**C5. Release mechanics are thin.**
- T1.3 runs `go test .` only. The fork's `ssq-ci.yml` runs `go test ./...`. Run the same locally, or wait for CI before tagging.
- The `replace` line sits at `go.mod:232`, and the `require` at `go.mod:31` stays `v5.19.3`.
- Only the root `go.mod` needs the bump: `tuitest`, `tools/scanner` and `tools/lint` do not reference go-git (grep returned nothing).
- T2.1 should specify `go.sum` entries and the new `go.sum` hashes, and that the tag must be fetchable via `GOPROXY` (the plan notes this in Risks but not in the task order).
- Also note that the Go module proxy caches a negative lookup if `go mod tidy` runs before the tag is pushed.

**C6. AC8 is nearly vacuous.**
- `TestReadEnrichment_RejectsIncludesAndUnsupported` (`session/git/backend/gogit/enrichment_test.go:49-60`) calls `ReadEnrichment` on a bare `.git/config` and never calls `git.Open`.
- It passes with or without the fork change.
- The behavior AC8 is really about is `layout.go:331-338` and `layout.go:379-383` (`vetExtension`).
  - V0 with any extension, or V1 with truthy `worktreeConfig`, falls back to the CLI.
  - I checked: truthy `worktreeConfig` still takes the `fallbackEligible` path, so the in-process backend's behavior does not change for truthy values.
  - The only change is that `worktreeConfig=false` on V1 no longer fails in `git.Open` at `repoview.go:85`, which is harmless.
- Consider adding a test at the `vetRepoConfig` or `Open` level so AC8 means something.

**C7. Other `git.Open` call sites change behavior with no test.**
- The only caller of `verifyExtensions` is `repository.go:216` (`Open`).
- Every go-git open in stapler-squad therefore changes behavior: `native.OpenRepo` (`session/git/native/open.go:29`), `OpenRepoKeepDescriptors` (`open.go:47`), `session/unfinished/gogitstore/open.go:45`, and `session/git/backend/gogit/repoview.go:85`.
- go-git ignores `config.worktree`, while stapler-squad's own `session/git/native/gitconfig/resolver.go:299-311` honors it.
- A per-worktree `core.bare`, `core.worktree`, or `core.sparseCheckout` is invisible to the go-git paths. The plan's Risks section mentions this but says "acceptable" without checking which callers read config:
  - `session/git/ops.go:302` is `IsCommitOnMain`.
  - `session/git/diff.go:109` is the diff path. If it reads status in a sparse-checkout worktree, results could be wrong.
- Add a short audit table of the callers that read worktree-sensitive state (status and diff, not only refs).

## MINOR

- **M1.** Requirements say the backlog ACs are "see the backlog item". The plan references them by number (AC1-11) but never lists them. Inline the one-line ACs in the plan for traceability. This also would have surfaced B1 earlier.
- **M2.** T2.2 says to verify the tests fail on ssq.1. It should say how. Add `go test -run` output captured before the bump, or commit the test before the `go.mod` change.
- **M3.** Story 3 (AC11) is correctly flagged as post-deploy. The grep should use the log line text `does not support extension: worktreeconfig`, with a timestamp filter, since `logs/staplersquad.log` is JSON lines (see CLAUDE.md "Application Data").
- **M4.** `research/stack.md` cites "tip 98880159, admin access verified". Re-verify the branch tip before pushing, because the tag must be created from the tip that CI tested.
- **M5.** The fork test named in AC6 (`TestOpenRepo_should_Succeed_When_V0RepoHasWorktreeConfigExtension`) should use a real on-disk repo and `PlainOpenWithOptions`, not `memory.NewStorage()`. The existing `TestVerifyExtensions` uses memory storage and would not catch `DetectDotGit` or `EnableDotGitCommonDir` interactions.

## What holds up
- Root cause is correct and verified: lowercase lookups against camelCase map keys (V0 gate), plus `worktreeconfig` missing from `builtinExtensions` (second gate). Both defects are real, and fixing only one leaves the failure.
- `verifyExtensions` has exactly one caller (`repository.go:216`), so there are no other extension-check call sites to patch in the fork.
- stapler-squad has a single open wrapper (`session/git/native/open.go`), with `norawgitopen` lint enforcing it. No separate V0 guard exists that would block the fixed open.
- After the fix, V0 and V1 repos with `worktreeConfig=true` open and resolve branch refs (probe above).
