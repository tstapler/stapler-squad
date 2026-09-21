# ADR-001: Atomic write-temp+rename protocol for `.git/worktrees/<name>/` admin files and conflicted-index writes

**Status**: Accepted
**Date**: 2026-09-07
**Project**: go-git-worktree-and-merge

## Context

`research/pitfalls.md` §1.4 verified directly against the pinned `go-git v5.19.2`
source that neither of its two relevant write paths is crash-safe or
real-git-lock-compatible:

- `storage/filesystem/dotgit/dotgit.go`'s `IndexWriter()` is a plain
  truncating `fs.Create` — no `index.lock`, no temp file, no fsync-then-rename.
  A crash mid-`Encode` leaves `index` truncated/structurally invalid in place,
  which real git cannot auto-recover (`rm -f .git/index && git reset` is the
  only remediation, and it discards any staged-but-uncommitted state).
- `storage/filesystem/dotgit/dotgit_setref.go`'s `setRefRwfs` takes an
  advisory `flock` and truncates+writes a ref file in place — a real `git`
  CLI process does not participate in that `flock` at all (git's own
  lockfile protocol is presence-of-a-`.lock`-file, not `flock`), so this path
  offers **zero** actual mutual exclusion against a concurrent real `git`
  process, and a crash between truncate and write leaves a zero-byte ref.

`research/stack.md` §2.2 and `research/pitfalls.md` §3/§5 independently
confirm real git's own protocol: `HEAD` is written through the ref-store API
(temp-file-then-atomic-rename, in effect), and the four
`.git/worktrees/<name>/` admin files are written in a specific,
crash-recoverable order (`gitdir` before `commondir`, `locked` written first
and removed last) that real git's own `list`/`prune` tooling already knows
how to interpret if a crash lands between any two steps.

This project's own success metric requires worktrees this project writes to
be indistinguishable from real `git worktree add` output to any other tool
that inspects them (real `git`, `gh`, a human) — so both the failure-state
shape (what a crash leaves behind) and the recovery-tool compatibility (does
`git worktree prune` recognize it) matter, not just "does it not corrupt
data."

## Decision

Build a project-owned atomic-write primitive, `AdminFileWriter` (see
`session/git/native_admin_writer.go` in the implementation plan), used for
**every** file this project writes under `.git/worktrees/<name>/`, the
linked worktree's own `WorktreeRedirectFile` (the `.git` file at the
worktree path itself — outside the admin dir, but subject to the identical
crash-mid-write risk, since a torn write there is not something real git's
`worktree list --porcelain` can detect or recover, as it inspects the
admin-dir side, not this file's content), and for the conflicted-index write
on the merge path:

1. Write content to a temp file in the **same directory** as the target
   (`<target>.tmp-<random-suffix>`) — same filesystem guarantees the
   subsequent rename is atomic.
2. `fsync` the temp file's file descriptor before closing it.
3. `os.Rename(tmp, target)` — POSIX atomic rename; the target either has the
   old content or the fully-written new content, never a torn write.
4. `fsync` the containing directory's file descriptor after the rename
   (directory-entry durability — without this, a rename can be lost on power
   loss even though the file's own `fsync` succeeded).

For multi-file admin sequences (`Add`), the **order** of these atomic writes
still follows real git's own order (`locked` written first with content
`"initializing"`, then `gitdir`, then `commondir`, then `HEAD`, then the
worktree's `.git` redirect file, then checkout populates `index` and the
working tree, then `locked` is removed last on success) — atomicity per file
is necessary but not sufficient; the sequence must also match what real
git's `should_prune_worktree()` already knows how to recognize as a
recoverable partial state.

This is a project-owned layer, not a go-git contribution or a fork of
go-git's `SetIndex`/`SetReference` — go-git's own maintainers have not
prioritized this (`research/pitfalls.md` §1.4's contrast: `packed-refs`
rewriting already uses temp+rename in go-git, `index`/loose-ref writes do
not, showing this is a known, unaddressed gap in the library itself, not an
oversight this project can wait out).

## Alternatives Considered

1. **Use go-git's `Storer.SetIndex`/`Storer.SetReference` as-is.** Rejected:
   verified non-atomic and non-interoperable with real git's own lock
   convention (see Context). Using it would silently reintroduce the exact
   corruption class this project's Feasibility Risks section names as the
   single highest-risk item.
2. **Replicate real git's exact `<file>.lock`-file-presence sentinel
   (`index.lock`, `<ref>.lock`) so a concurrent real `git` CLI process
   naturally recognizes an in-progress write via `.lock` and defers/errors.**
   Deferred, not adopted for v1: this project's actual concurrent-writer risk
   is other **stapler-squad code** (other sessions, backlog automation)
   racing on the same repo, and `WithRepoWorktreeLock` (existing,
   `session/git/worktree_lock.go`) already serializes every
   stapler-squad-issued write against every other stapler-squad-issued write
   for a given repo path — this project's own writes never race each other.
   The only *other* process this repo's own scope allows to touch the same
   worktree concurrently is `FetchBranch`/`CheckoutBranch` (subprocess `git`,
   explicitly out of scope for this project) and `gh` CLI — neither of which
   writes `.git/worktrees/<name>/` admin files or the index. A real
   `.lock`-file sentinel would only matter if an *external*, unrelated `git`
   process could race this project's writes to the *same specific file*,
   which the current call-site audit (`research/architecture.md` §1) found
   no evidence of. Recorded as a design note for a future project if that
   assumption changes, not built now — building unused interop machinery
   would be premature generality.
3. **Delegate crash-safety entirely to `WithRepoWorktreeLock`, skip the
   temp+rename layer, and rely on the lock to prevent any read during a
   write.** Rejected: the lock only prevents *this project's own* concurrent
   operations from interleaving. It does nothing for a process crash
   mid-write (the lock is released, or was never held by whatever inspects
   the file next — e.g. the next session's own `Setup()` call, minutes
   later, or a human running `git worktree list` from a terminal). Atomicity
   of the write itself is a distinct property from mutual exclusion between
   writers, and this project needs both.

## Consequences

- New code: `session/git/native_admin_writer.go` (the `AdminFileWriter`
  primitive) is a foundational, first-implemented piece — every later
  worktree-add/remove/prune and merge-conflict task depends on it.
- Every native admin-file write in this project's Add/Remove/Prune/Merge
  code paths must go through `AdminFileWriter`, never a bare
  `os.WriteFile`/`os.Create` — this is enforced by code review and the
  `code-hotspot-analysis`/`quality:reflect-and-fix` gates, not by a compiler
  check (Go has no way to forbid `os.WriteFile` at the type level without a
  linter rule, which is out of scope for this project to add).
- A crash mid-`Add`/mid-conflict-write leaves the same partial state real
  git's own crash would leave (per the ordering above), so real git's
  `worktree prune`/`list` tooling, and this project's own `nativeWorktreePrune`,
  already know how to recognize and clean it up — no bespoke recovery code
  is needed beyond what `worktreeAlreadyRegisteredForBranch`'s existing
  `locked`-file check already does.

## Update: Re-examination of Alternative #2's deferral (plan-repair, 2026-09-07)

`research/architecture.md` §2(a) and `research/pitfalls.md` §1.4 reached
contradictory conclusions about whether go-git's ref writes provide real
cross-process mutual exclusion against a concurrent real `git` CLI process.
`pitfalls.md`'s conclusion governs: it is a direct read of the pinned
`v5.19.2` source (`setRefRwfs` takes an advisory `flock`, which a real `git`
process does not participate in at all — git's own lockfile protocol is
presence-of-a-`.lock`-file, not `flock`). `architecture.md`'s contrary claim
rested on a test that raced two real `git` processes against each other,
never a go-git writer against a real `git` writer, so it does not actually
support the claim it makes.

This does not overturn Alternative #2's deferral wholesale — it narrows it to
per-ref-write-site analysis:

- **Fresh branch-ref creation** (worktree `Add`, Task 2.1.2a): deferral
  **stands**. The ref does not exist until this project's own code creates
  it; no in-scope real `git` subprocess (`FetchBranch`, `CheckoutBranch`, `gh`
  CLI) has a reason to write a branch ref before this project creates it, and
  `WithRepoWorktreeLock` already serializes every stapler-squad-issued writer
  against it.
- **Merge ref-advance on an existing branch ref** (Task 3.4.1b): deferral is
  **reversed**. This ref is exactly the kind of already-existing,
  concurrently-touchable ref `FetchBranch`/`CheckoutBranch`/a fix-agent's own
  `git merge`/`git rebase` subprocess can plausibly write to during the same
  session's lifetime (`requirements.md`'s Users/Consumers section). This
  write now goes through `writeRefWithLockSentinel`
  (`session/git/native_merge.go`, Task 2.5.3b) — a project-owned
  `<ref>.lock`-file-presence sentinel matching real git's own ref-write
  protocol, not go-git's `SetReference`. See `implementation/plan.md` Epic
  2.5, Story 2.5.3 for the race test proving this is necessary and
  sufficient.
