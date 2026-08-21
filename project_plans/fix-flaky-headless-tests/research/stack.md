# Research: Technology Stack — Agent 1

Scope: portable, idiomatic Go patterns for (1) invoking a temp shell-script test
fixture without depending on OS-honored exec-bit / Gatekeeper approval, and (2)
robustly classifying subprocess crash signals (SIGBUS/SIGSEGV) vs clean exit.
Plus platform (Linux/macOS) differences relevant to both.

## 1. Invoking a fake-claude fixture script without relying on direct fork/exec-by-path

### Current pattern (confirmed in code)

- `session/headless/runner.go:129` — `ProcessRunner.Run` calls
  `executor.StartProcess(ctx, r.claudeBin, args, opts...)`, where `r.claudeBin`
  is a filesystem path to whatever script the test wrote.
- `executor.StartProcess` (`executor/managed_process.go:174`) builds the command
  via `safeexec.CommandContext(derived, name, args...)`.
- `safeexec.CommandContext` (`executor/safeexec/safeexec.go:30`) is a thin
  wrapper: `cmd := exec.CommandContext(ctx, name, arg...)` — it only adds
  `WaitDelay`, nothing exec-permission-related.
- `exec.Command`/`exec.CommandContext`: when `name` contains a path separator
  (as `claudePath`/`scriptPath` always do — they're `filepath.Join(dir, ...)`),
  Go does **not** consult `PATH` at all; it execs that literal path directly via
  `syscall.Exec`/`posix_spawn`. This is the direct-exec-by-path pattern named in
  requirements.md.
- All three test files write the fixture with `os.WriteFile(path, script,
  0o755)` and then hand that exact path to the runner as `claudeBin`:
  - `session/headless/capability_check_test.go:25` (`writeCapabilityCheckFakeClaudeScript`)
    and `:170` (`writeSlowCapabilityCheckFakeClaudeScript`)
  - `session/headless/pool_test.go:170` and `:691`
  - `session/headless/caller_test.go:39,54,86,101,105`

None of these pass through an interpreter explicitly — the shebang line
(`#!/bin/sh`) is what makes execution work, and shebang-based direct-exec is
exactly the invocation style Gatekeeper/EDR/MDM tooling can intercept for a
freshly-written, freshly-chmod'd file (see §3).

### Idiomatic fix: invoke via an explicit interpreter

Replace direct-exec-by-path with `exec.Command("sh", scriptPath, ...)` (or
thread an interpreter arg through `ProcessRunner`/`NewProcessRunnerForTesting`
so the test helper can do `NewProcessRunnerForTesting("sh")` +
`args := append([]string{scriptPath}, realArgs...)`). This is strictly more
portable than shebang-based exec because:

- `sh` (or `/bin/sh`) is a long-lived, already-trusted, code-signed system
  binary on both Linux and macOS — it is never the "freshly written unsigned
  file" Gatekeeper/quarantine/EDR heuristics target. The *script* becomes a
  passive text argument/file read by `sh`, not something the OS itself has to
  approve for execution.
- It removes any dependency on the exec bit at all — `sh <path>` runs `<path>`
  through the interpreter's read+parse path regardless of whether the file's
  mode bits include `+x`. `os.WriteFile(..., 0o755)` becomes unnecessary for
  correctness (though harmless to keep for clarity/defense-in-depth).
- It is the same idiom Go's own `os/exec` test suite and most subprocess-fixture
  patterns in the wild use for portability — never rely on shebang dispatch
  for test fixtures whose only job is to be a stand-in binary.

Concretely, for this codebase, the two call sites needing a change are:
1. **Production code path is fine as-is** — `ProcessRunner.Run` genuinely
   execs the real `claude` CLI by path in production, which *is* meant to be
   a trusted, user-installed, presumably-signed binary; no change needed there
   per the "no change to `executor.StartProcess`/`safeexec` beyond what's
   needed for the fake-claude test-script invocation path" scope note.
2. **Test-only fixture construction** — `NewProcessRunnerForTesting` (used by
   all 3 clusters) is the natural seam: either (a) have it accept an
   interpreter + script path and construct the args so `StartProcess` execs
   `sh` with the script path as `args[0]`, or (b) keep the shebang script but
   make `NewProcessRunnerForTesting` resolve and pass `"sh"` as `claudeBin`
   with the script path prepended to every call's `args`. Both are
   `ProcessRunner`-only changes — no change to `executor.StartProcess`/
   `safeexec` itself.

### Alternative considered: skip/diagnose instead of failing on exec refusal

Acceptance criterion 1 explicitly allows "a clear skip/diagnostic instead of a
confusing count-mismatch failure" as an alternative to full portability. This
is strictly weaker (still leaves the tests unable to actually verify the code
path on an affected machine) but is a valid fallback if threading an
interpreter through `ProcessRunner` turns out to be more invasive than
expected once Agent 3 (architecture) traces the actual call graph. Recommend
Agent 3 assess feasibility; the interpreter-invocation fix is preferred
because it makes the tests **work**, not just fail cleanly.

## 2. Classifying subprocess crash (SIGBUS/SIGSEGV) vs clean exit — stdlib facilities

### Current pattern (confirmed in code)

`session/unfinished/gogitstore/mmap_truncation_test.go:125-136` runs the
crash-inducing code in a subprocess via `safeexec.CommandContext(...).
CombinedOutput()`, then classifies the outcome purely by `bytes.Contains(out,
[]byte("..."))` string matching against markers the helper process itself
prints (`ORDINARY_RECOVER_CAUGHT_IT`, `NO_FAULT_OCCURRED`, `PANIC_RECOVERED`,
`SURVIVED`), falling back to "err != nil → assume it's the expected crash, log
and return" with no structural check of *why* the process died.

### More robust classification available in stdlib (confirmed via `go doc`, go1.26.4 toolchain in this repo)

- `cmd.CombinedOutput()` (and `Run`/`Output`) return an `*exec.ExitError` when
  the process exits non-zero **or is killed by a signal**. `exec.ExitError`
  embeds `*os.ProcessState`.
- `os.ProcessState` exposes `.ExitCode()` (returns `-1` if terminated by
  signal, not exited normally — this alone already distinguishes "clean exit"
  from "died abnormally" without string parsing) and `.Sys() any`.
- On Unix (Linux and macOS both — `GOOS=linux`/`GOOS=darwin` share the unix
  build tag for this), `ProcessState.Sys()` can be type-asserted to
  `syscall.WaitStatus`, which exposes:
  - `.Signaled() bool` — true iff the process was killed by a signal (vs.
    exited via `exit()`/`return` from `main`)
  - `.Signal() syscall.Signal` — the actual signal number/name; comparable
    directly to `syscall.SIGBUS` / `syscall.SIGSEGV` constants, and
    `.String()`-able for logging (e.g. `"bus error"`, `"segmentation
    violation"`) without needing the helper process to self-report a marker
    string at all.
  - `.CoreDump() bool` — whether a core was generated, an extra corroborating
    signal.

  This is portable across Linux/macOS because `syscall.WaitStatus` and its
  methods are defined for both (unix build constraint), even though the
  underlying `wait4`/`waitpid` syscall representations differ in bit layout —
  the Go stdlib abstracts that away identically on both platforms. (It would
  *not* be portable to Windows, but this test already gates on `git` being on
  `PATH` and models POSIX mmap semantics, so Windows portability is out of
  scope here.)

### Recommended tightening

Replace/augment the current `bytes.Contains(out, ...)` marker-string branch
with a structural check:

```go
if err != nil {
    var exitErr *exec.ExitError
    if errors.As(err, &exitErr) {
        if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
            sig := ws.Signal()
            t.Logf("subprocess killed by signal %v (expected — this IS the point of the test)", sig)
            return
        }
    }
    t.Logf("subprocess did not exit cleanly (expected): err=%v\noutput:\n%s", err, out)
    return
}
```

This keeps the existing marker-string checks for the *non-faulting* branch
(where the process legitimately exits 0 and self-reports which code path it
took — that part isn't a crash-classification problem, it's the helper
communicating its own internal control flow, which has no better channel than
stdout in a `go test` subprocess harness) but replaces the "assume any non-nil
`err` is the expected crash" leap with an actual signal check. This closes a
real gap: today, *any* subprocess failure — a panic that Go's runtime itself
reports as exit code 2, an `os.Exit(1)` from unrelated test-framework
plumbing, a `t.Fatal` in the helper before the truncation even happens — is
currently indistinguishable from the intended SIGBUS/SIGSEGV and is silently
accepted as "expected." Checking `ws.Signaled() && (ws.Signal() ==
syscall.SIGBUS || ws.Signal() == syscall.SIGSEGV)` specifically would make
acceptance criterion 3's "correctly classified" bar concrete — see Agent 4
(pitfalls) for whether this gap is worth closing given the test's own
documented "undefined behavior, tolerant of either outcome" design intent.

### `runtime/debug.SetPanicOnFault` — status check

`debug.SetPanicOnFault` is already used correctly in this test file exactly as
documented (per-goroutine, sticky, converts a would-be-fatal SIGBUS/SIGSEGV
into a recoverable Go panic) — this is confirmed current, idiomatic API, no
newer/better facility exists in go1.26.4 for this specific purpose. No stdlib
alternative supersedes it; the file's own doc comment (lines 27-38) already
correctly scopes it as available-but-not-applied-in-production, which matches
current guidance (it's a debugging/defensive tool, not something to enable
blanket in production hot paths without measuring the cost, exactly as the
comment states re: `FindOffset`'s hot path).

## 3. Linux vs macOS exec/signal semantics relevant to these two clusters

### Cluster 1+2 (exec-permission refusal)

- **Quarantine xattr (`com.apple.quarantine`) — does `os.WriteFile` from a Go
  process set it?** No. The quarantine xattr is applied by specific
  Apple frameworks/apps that have `LSFileQuarantineEnabled` and explicitly
  tag files as coming from "the internet" or an untrusted source — e.g.
  Safari, Mail, Chrome, `curl` when invoked through certain flows, or an app
  explicitly calling the quarantine APIs. A plain Go process calling
  `os.WriteFile`/`ioutil.WriteFile` (a bare `write(2)`/`open(2)` syscall
  sequence) does **not** set `com.apple.quarantine` — confirmed by the
  general mechanism (quarantine is opt-in per-writer, not global to all file
  creation) and consistent with community reports of the same
  write-then-exec-in-Go pattern failing for **other** reasons (see below), not
  quarantine specifically, since a `go test` binary creating its own temp
  fixture was never "downloaded."
- **What *does* cause `fork/exec ...: operation not permitted` for
  freshly-written scripts on macOS, then?** Multiple non-Gatekeeper-quarantine
  causes are documented in the wild for exactly this Go pattern:
  - **Endpoint security / EDR / MDM agents** (CrowdStrike, SentinelOne,
    Jamf-managed security profiles, corporate "Full Disk Access"/System
    Extension-based monitors) hook `exec`/`fork` via the Endpoint Security
    framework and can deny newly-created, unsigned, unnotarized executables
    as a heuristic — this is the most likely cause for a corporate/managed
    macOS laptop (matches the requirements doc's inference that the user
    "also works" on macOS).
  - **TCC (Transparency, Consent, and Control)** can deny a parent process
    (Terminal.app, iTerm2, or the IDE running `go test`) permission to spawn
    children touching certain protected paths, surfacing as the same EPERM.
  - A documented real-world case (`evilmartians/lefthook#157`, "Running any
    command on macOS fails with 'operation not permitted'") shows this exact
    Go `fork/exec` EPERM class, unrelated to quarantine, tied to Go's
    fork+exec startup path interacting with macOS's copy-on-fork behavior
    for large address spaces / security-software-injected libraries.
  - Gatekeeper *can* still be a contributing factor if the fixture is ever
    written by a tool that itself sets quarantine (unlikely for `go test`,
    but worth ruling out with `xattr -l <path>` on an affected macOS machine
    as a diagnostic step — see acceptance criterion 6, this needs macOS
    confirmation).
  - **Net effect on the fix**: regardless of which of these is the actual
    cause on the reporter's machine, invoking via `sh <path>` (§1) sidesteps
    all of them simultaneously, because the thing being exec'd is always the
    pre-existing, already-trusted `/bin/sh`, never the freshly-written file.
    This is why the interpreter-invocation fix is robust even without
    pinning down the exact denial mechanism — confirming the precise
    mechanism is a nice-to-have diagnostic, not a blocker for the fix.
- **`/tmp` handling differences**: `t.TempDir()` on macOS resolves under
  `$TMPDIR` (typically `/var/folders/...`, per-user, not world-writable
  `/tmp`) vs Linux's `/tmp` (often `os.TempDir()` → `/tmp` directly, or
  `$TMPDIR` if set). This difference is not itself a permission-refusal
  cause — both are writable and executable by the owning user by default on
  stock installs — but corporate macOS MDM profiles occasionally apply
  "noexec"-equivalent restrictions or additional EDR scrutiny specifically to
  paths outside an allowlist, which could compound the exec-refusal risk in
  `$TMPDIR` on a managed machine. Not verifiable from this (Linux) session;
  flagged for macOS confirmation per acceptance criterion 6.

### Cluster 3 (SIGBUS/SIGSEGV on truncated mmap read)

- This is genuinely platform/kernel-dependent **by design**, as the test's own
  extensive doc comments already state (lines 98-113, 139-159): reading past
  the new EOF of a truncated-but-still-mapped file is undefined behavior at
  the OS level.
  - **Linux**: reliably delivers `SIGBUS` for reads past the mapped file's
    current EOF within an existing mapping (confirmed empirically in this
    session — 5/5 runs faulted, per requirements.md's pre-planning table).
  - **macOS (xnu/Mach)**: same POSIX `mmap` contract applies, but the
    underlying VM subsystem's handling of a hole/beyond-EOF page fault after
    truncation is not guaranteed identical to Linux's; the test's own
    tolerant branch (`NO_FAULT_OCCURRED`) exists precisely because this can
    differ by kernel/filesystem combination.
- No portable Go stdlib API can force deterministic behavior here — the fix
  available (§2) is about classifying *whichever* outcome actually occurs
  more precisely, not about making the fault itself deterministic across
  platforms.

## Sources

- [lefthook#157 — "Running any command on MacOS fails with an error 'operation not permitted'"](https://github.com/evilmartians/lefthook/issues/157)
- [Script on Mac, operation not permitted. Quarantine — Mac O'Clock](https://medium.com/macoclock/script-on-mac-operation-not-permitted-quarantine-3388124b9a4d)
- [golang-nuts — "[Macos/Apple M1] some fork/exec … operation not permitted error"](https://groups.google.com/d/topic/golang-nuts/Ul1XsHm5I2s)
- [golang-nuts — "fork/exec /bin/bash: operation not permitted"](https://groups.google.com/g/golang-nuts/c/pkJKF6m7Xgg)
- `go doc os/exec.ExitError`, `go doc os.ProcessState`, `go doc syscall.WaitStatus` (this repo's go1.26.4 toolchain, run directly, VERIFIED)
- In-repo: `session/headless/runner.go:129`, `executor/managed_process.go:156-182`,
  `executor/safeexec/safeexec.go:30`, `session/headless/capability_check_test.go`,
  `session/headless/pool_test.go`, `session/headless/caller_test.go`,
  `session/unfinished/gogitstore/mmap_truncation_test.go` (all read directly, VERIFIED)
