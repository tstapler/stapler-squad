# Troubleshoot Bazel

One entry per error state. The wrapper (`tools/bazel`) prints the same one-line remedy when it recognises the
failure; the hint text below is quoted from `tools/bzl/errors.yaml`, the only place it is written. Run
`make bazel-doctor` first when the cause is not obvious. States 6, 7 and 9 quote the wrapper hints; states 5, 8 and 11
have no `errors.yaml` row.

### State 1: Stale BUILD file

Symptom (from `tools/bzl/testdata/errors/E1.log`): `compilepkg: missing strict dependencies:` or
`undefined: <pkg>` after an import change, or `no such file` for a deleted source.

<!-- errors.yaml:E1 -->
bazel-hint: a BUILD file is out of date with the Go sources. Remedy: run 'make gazelle', then commit the BUILD diff.

Cheap pre-check without Bazel: `bash tools/bzl/check-build-fresh-cheap.sh`. The authoritative gate is
`make gazelle-check` (Gazelle `-mode=diff`).

### State 2: Missing external dependency

Symptom (captured from a real run, `tools/bzl/testdata/errors/E2.log`):
`ERROR: Repository '@@gazelle++go_deps+com_example_nosuch' is not defined`; older Bazel also prints
`no such package '@@gazelle++go_deps...` or `repository ... not found`.

<!-- errors.yaml:E2 -->
bazel-hint: a Go dependency is not wired into MODULE.bazel. Remedy: run 'go get <module> && go mod tidy && make gazelle && bazel mod tidy'.


### State 3: Generated code absent or misnamed

Two different failures share this section.

- Plain `go`, gopls or the editor cannot find `gen/` or `session/ent` packages (symptom,
  `tools/bzl/testdata/errors/E3.log`: `cannot find package ".../gen/proto/go/session/v1"`). This inverts the old
  `make proto-gen ent-gen` precondition: plain `go` needs generated code, `bazel build` does not.

<!-- errors.yaml:E3 -->
bazel-hint: plain go and gopls need generated code; Bazel does not. Remedy: run 'make sync' (or use 'make go-build' / 'make go-test', which sync first).

- Bazel itself cannot find a generated-code label (captured from a real run, `tools/bzl/testdata/errors/E10.log`:
  `no such target '//proto/session/v1:...' ... not declared in package`).

<!-- errors.yaml:E10 -->
bazel-hint: a BUILD label for generated code is missing or misnamed; 'make sync' will not help because Bazel does not read gen/. Remedy: run 'make gazelle', then check //proto/session/v1 and //tools/bzl/ent (stale ent package list: 'bazel run //tools/bzl/ent:update_packages').


### State 4: Sandbox failure

Symptom (`tools/bzl/testdata/errors/E8.log`): `permission denied` or `no such file` for a path outside the runfiles,
on a line that names `linux-sandbox`, `sandbox-exec`, `/sandbox/` or `execroot`.

<!-- errors.yaml:E8 -->
bazel-hint: a test touched something outside its sandbox. Remedy: fix the test to use TEST_TMPDIR or runfiles (see docs/how-to/troubleshoot-bazel.md#state-4); never tag it no-sandbox without a reason comment and an issue.

Reproduce with `bazel test <target> --sandbox_debug --test_output=streamed --nocache_test_results`.

### State 5: Cache corruption or a poisoned local cache

Symptom (not reproduced on this host; from `research/ux.md` section 5, so treat the exact strings as unconfirmed):
a checksum or digest mismatch, outputs that look stale, or "worked on CI, fails locally".

Remedy, least destructive first: `bazel clean`; if it persists, delete the disk cache directory
`~/.cache/stapler-squad-bazel/disk` (content-addressed, safe to remove, rebuilt on demand); last resort
`bazel clean --expunge`. Do not delete the repository cache casually, it forces every external archive to download again.
There is no `errors.yaml` row for this state because no failure line identifies it reliably.

### State 6: Bazel server out of memory or killed

Symptom (seeded fixture `tools/bzl/testdata/errors/E4.log`, not a captured run):
`Server terminated abruptly (error code: 14, error message: 'Socket closed', ...)`, or a bare `Killed` with exit 137
when the kernel OOM killer fired. Other agent sessions on the host can die in the same event.

<!-- errors.yaml:E4 -->
bazel-hint: the Bazel server ran out of memory or was OOM-killed. Remedy: wait for other Bazel runs to finish, run 'bazel shutdown', then retry; do not raise --jobs.

Check `free -g` and `uptime` first; `make bazel-doctor` check 7 reports memory and load. The wrapper already caps each
run (`BAZEL_WRAPPER_MEMORY_MAX`, default 14G) and the backlog work limit is two concurrent sessions. Background:
[protect-live-service-from-bazel](protect-live-service-from-bazel.md) and the `bazel-perf` skill.

### State 7: Out of disk

Symptom (seeded fixture `tools/bzl/testdata/errors/E5.log`, not a captured run): `... (No space left on device)` in an
action, the output base, the disk cache or `/tmp`.

<!-- errors.yaml:E5 -->
bazel-hint: the disk is full. Remedy: run 'bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh' first (dry run: lists orphan output bases, the disk cache size against its GC cap, and stale /tmp test directories, with sizes; removes nothing), review the list, re-run it with --apply, then retry.

Reclaim order: orphan output bases (the script above), then the disk cache (its GC only runs while a server is idle;
`bash tools/bzl/bazel-disk-alarm.sh` names the breached path), then `bazel clean` in finished worktrees, then
`bazel clean --expunge`. Long temp paths also break unix-socket tests; keep scratch short and on disk, not on a
RAM-backed `/tmp`.

### State 8: Bazel version drift

Symptom (from `tools/bzl/doctor.sh` check 1, which prints it when the versions differ):
`1 version: Bazel <found> does not match .bazelversion (<wanted>); run through bazelisk`. A different `bazel` on
`PATH` also shows up as rule APIs missing or `name '<x>' is not defined` in a `.bzl` file.

Remedy: run `make bazel-doctor`, and call Bazel through Bazelisk so it honours `.bazelversion`. Version bumps follow the
[upgrade runbook](upgrade-bazel.md) and the `bazel-version-upgrades` skill.

### State 9: Another command holds the lock

Symptom (seeded fixture `tools/bzl/testdata/errors/E9.log`, not a captured run):
`Another command (pid=12345) is running. Waiting for it to complete on the server (server_pid=23456)...`

<!-- errors.yaml:E9 -->
bazel-hint: another Bazel command holds this worktree's lock; waiting is normal. Remedy: wait, or use a separate worktree.

This is expected, not an error: each output base runs one command at a time, so two agents in one worktree serialise.
Do not kill the holder. The host-wide queue is a different thing: its wait message is in
[first-build-in-a-fresh-worktree](first-build-in-a-fresh-worktree.md#3-when-other-bazel-runs-hold-the-slots).

### State 10: Remote cache unreachable or credentials rejected

Symptom (`tools/bzl/testdata/errors/E6.log`, `E11.log`): a remote-cache `UNAVAILABLE` / `Connection refused`
warning, or `HTTP/1.1 403 Forbidden` / `401 Unauthorized`. Both leave the build correct but slower.

<!-- errors.yaml:E6 -->
bazel-hint: remote cache unreachable, building locally (results are correct, the build is slower). Remedy: none needed; run 'make bazel-doctor' to check the cache.

<!-- errors.yaml:E11 -->
!! bazel-hint: the remote cache REJECTED our credentials (HTTP 401/403 or expired), so the shared cache is not being used. Remedy: refresh the credential ('make bazel-doctor' names which one); builds stay correct but slow.

`make bazel-doctor` check 2 probes the cache and names which of the two applies.

### State 11: macOS binary unsigned after a build

Symptom (not reproducible on this Linux host; a device-verified item per the requirements): macOS shows the TCC
permission prompts again after a rebuild, because Bazel outputs are not codesigned.

Remedy: the install path signs a copy of the Bazel output at a stable path (`make _codesign-binary`, see
`docs/how-to/macos-codesigning.md`); never sign or run the binary in `bazel-bin` directly.
