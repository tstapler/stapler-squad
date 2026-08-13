# Architecture Review: ssh-remote-workspaces
**Date**: 2026-08-06
**Verdict**: RESOLVED (2026-08-06 validation pass)

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo — no Constitution
Violations section is included.

## Blockers

- [x] **RESOLVED** — Task 4.2.1a now defines `ExecutionTarget` as a sum type
  (`LocalTarget{}` / `RemoteExecutionTarget{Target, Runner}`) held as `Instance`'s single field,
  constructed atomically in Task 4.2.1c and consumed via `.Runner()`/`.IsRemote()` everywhere
  (env wrapping Task 2.3.1b, hook routing Task 5.2.1a, approval relay attach Epic 5.1) instead
  of two independently-settable fields. This makes the "remote target set but local runner
  installed" disagreement state unrepresentable, per the remediation this blocker recommended.
  **Story 4.2.1 / Epic 5.2 (`session/instance.go`, `server/services/hook_injector.go`,
  `session/tmux/remote_env.go`) — "is this session remote" is decided independently in at
  least three places with no type-level guarantee they agree.** `Instance.RemoteTarget
  *RemoteTarget` (Task 4.2.1a) and the `CommandRunner` actually installed on that instance's
  `TmuxSession`/`GitWorktree` (Task 4.2.1d) are two separately-settable fields — nothing stops
  a future construction path from setting one without the other. Downstream, at least three
  independent call sites re-derive "is this remote" from different signals: Task 2.3.1b's env
  wrapper ("type-switch or a `CommandRunner`-level `IsRemote() bool` method"), Task 5.2.1a's
  hook-command branch ("reads `Instance.RemoteTarget`"), and Epic 5.1's approval relay
  attachment. If any construction path or future refactor lets these diverge — e.g. an
  `Instance` with `RemoteTarget` set but a `LocalRunner` still installed, or vice versa —
  `InjectHookConfig` routes approval hooks to the wrong destination (`localhost:8543` instead
  of the relay socket, or the relay socket for a session actually running locally) *silently*:
  no error, just a hook that hangs or targets nothing. This is exactly the class of bug
  type-driven design's "make illegal states unrepresentable" targets, and it sits on the
  security-relevant approval path ADR-003 depends on.
  **Remediation**: collapse the pair into one type constructed atomically — e.g. an
  `ExecutionTarget` sum type (`LocalTarget{}` / `RemoteExecutionTarget{Target RemoteTarget,
  Runner *SSHRunner}`) that `Instance` holds a single field of, with `Runner()`/`IsRemote()`
  as methods on it rather than independently-checkable state. Every current "is remote" branch
  (env wrapping, hook routing, approval relay attach) should read this one field, not
  `Instance.RemoteTarget != nil` in one place and a `CommandRunner` type-switch in another.

## Concerns

- [ ] **Epic 1.3 (`session/git/worktree_git.go`), ADR-002 — `session/git` is made to depend on
  a `CommandRunner` interface (`Run` + `Start`) it only ever calls half of.** `runGitCommand`/
  `runExec` (Tasks 1.3.1b/c) only ever call `Run` — `session/git` has no persistent, piped-stdio
  use case, so it never calls `Start`. ADR-002 has `session/git` import the full
  `tmux.CommandRunner` anyway "rather than defining a second, near-identical interface." That
  reasoning is the interface-pollution-checklist's own smell #1 inverted: the checklist's
  corrective pattern is "define the interface where it's consumed, scoped to only the methods
  that consumer needs" — `session/git` consuming only `Run` is the textbook case for its own
  narrower interface, structurally satisfied by `tmux.LocalRunner`/`tmux.SSHRunner` with no
  import required (Go interfaces are structural). As written, it's also an avoidable same-module
  dependency from `session/git` onto `session/tmux` for a reason that has nothing to do with
  tmux.
  **Remediation**: `session/git` declares its own single-method interface (e.g. `type
  CommandRunner interface { Run(ctx context.Context, name string, args ...string) ([]byte,
  error) }`); `tmux.LocalRunner`/`tmux.SSHRunner` satisfy it for free, no import of
  `session/tmux` needed from `session/git`.

- [ ] **Epics 1.1–1.3, ADR-002 — the interface and both of its concrete implementations
  (`LocalRunner`, `SSHRunner`) are planned to live in the same package (`session/tmux`), and
  ADR-002's stated rationale for rejecting a shared package doesn't hold up.** ADR-002 cites
  `SessionStreamer` as this repo's precedent for "correct" interface placement — "defined in
  the consumer package... satisfied implicitly... no `implements` declaration needed," i.e. the
  implementer lives in a *different* package from the interface. `CommandRunner` doesn't follow
  that: `command_runner.go` (interface + `LocalRunner`) and `ssh_runner.go` (`SSHRunner`) are
  all planned for `session/tmux` — interface-pollution-checklist smell #2 ("interface defined
  next to its implementation") in its plainest form. ADR-002 rejects extracting a shared
  package (e.g. `session/exec`) as "speculative abstraction... no behavior of its own beyond
  the interface declaration," but that's not accurate for what would actually move there:
  `LocalRunner`, `SSHRunner` (dial/auth/reconnect/backoff/circuit-breaker state — Epic 2.1) is
  substantial concrete behavior, and the package would have two real consumers
  (`session/tmux`, `session/git`) from day one — the same two-consumer bar the
  interface-pollution-checklist itself uses to justify extraction elsewhere in this repo. As
  planned, `session/tmux` — a tmux-domain package — becomes the home of generic,
  tmux-unrelated remote-execution transport that a peer domain package must import, which is
  backwards from a Clean/Hexagonal-Architecture standpoint (domain package hosting shared
  infrastructure a sibling domain depends on).
  **Remediation**: extract `CommandRunner`/`LocalRunner`/`SSHRunner` (and the backoff/circuit
  state) into a package below both consumers (e.g. `session/remoterunner`), imported by
  `session/tmux` and `session/git` as peers. This also resolves the previous finding's import
  direction concern once `session/git` has its own narrower interface satisfied by the same
  concrete types.

- [ ] **Story 3.1.1 (`config/types.go`), Story 3.3.1 (`session/sshremote/known_hosts.go`) —
  `IdentityRef` and `HostKeyFingerprint` are documented invariants, not encoded ones.** The
  Domain Glossary states `IdentityRef` is "Opaque string... Never a raw key path or key byte
  content," but Task 3.1.1a types it as a plain `string` field on `RemoteConfig` alongside
  `Name`, `Host`, `User`, `BasePath` — all also plain strings. Nothing in the type system stops
  a future call site from passing a raw key path, another remote's `Name`, or an arbitrary
  string where an `IdentityRef` is expected; the invariant lives only in a doc comment. Same
  primitive-obsession gap applies to `HostKeyFingerprint` (also plain `string` per the
  glossary).
  **Remediation**: `type IdentityRef string` and `type HostKeyFingerprint string` (newtypes,
  not aliases) — near-zero implementation cost, and it makes "pass the wrong string" a compile
  error instead of a doc-comment-only convention, consistent with this codebase's existing
  `UserID`/similar newtype usage elsewhere.

- [ ] **Task 4.2.1c (`server/services/session_service.go`) — the initial SSH dial during
  `CreateSession` has no stated timeout, distinct from Epic 2.1.2's reconnect/backoff (which
  only covers *already-established* connections).** The acceptance criteria for Story 4.2.1
  cover "unknown remote_name" but not "remote configured, but host unreachable/firewalled at
  creation time." As written, an unreachable remote risks hanging the `CreateSession` RPC for
  as long as the underlying TCP/SSH handshake takes to fail (which, for a silently-dropping
  firewall, can be minutes, not seconds) — a reliability gap against the requirements'
  "network partition... must degrade gracefully" NFR, applied to the creation path rather than
  an established session.
  **Remediation**: wrap the dial in Task 4.2.1c with an explicit `context.WithTimeout` (short —
  session creation is interactive), and add an acceptance criterion + test for
  "remote configured but unreachable at creation time" alongside the existing unknown-name
  case.

## Nitpicks

- Story 3.2.1 (`session/sshremote/keystore.go`) plans to "mirror `github/keychain.go`'s exact
  ... wrapper shape" with its own separate `sync.Mutex`, rather than factoring the two
  structurally-identical mutex-wrapped `zalando/go-keyring` facades into one generic helper
  parameterized by service/key-prefix. Minor DRY miss, not blocking — it does correctly reuse
  a proven pattern rather than inventing a new one.
- `RemoteConnectionState`'s Go representation (typed-string-constant enum vs. a sealed
  interface/sum type) isn't specified anywhere in the plan. Given only three states today and
  no per-state payload, a typed-string-constant enum is adequate — call it out explicitly in
  the implementing task and add an exhaustiveness check (e.g. a test asserting the frontend
  `STATE_LABEL`/`STATE_ANNOUNCE` maps in `RemoteConnectionIndicator.tsx` cover every backend
  value) so the two sides can't silently drift.
- Tasks 1.2.1a and 1.3.1a both add a `runner CommandRunner` field defaulting to
  `LocalRunner{}`/`tmux.LocalRunner{}`, but the injection mechanism is left as "functional
  option or a wrapping constructor" in `session/tmux` without pinning down which, and Task
  1.3.1a doesn't specify either way for `session/git`. Worth deciding once, before
  implementation, so the two packages don't end up with two different injection idioms for the
  same seam.
