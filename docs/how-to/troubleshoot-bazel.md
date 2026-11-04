# Troubleshoot Bazel

One entry per error state. The wrapper (`tools/bazel`) prints the same one-line remedy when it recognises the
failure; the hint text below is quoted from `tools/bzl/errors.yaml`, the only place it is written. Run
`make bazel-doctor` first when the cause is not obvious. States 5-9 and 11 of the plan arrive with Task 11.3b
under the same headings.

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

### State 10: Remote cache unreachable or credentials rejected

Symptom (`tools/bzl/testdata/errors/E6.log`, `E11.log`): a remote-cache `UNAVAILABLE` / `Connection refused`
warning, or `HTTP/1.1 403 Forbidden` / `401 Unauthorized`. Both leave the build correct but slower.

<!-- errors.yaml:E6 -->
bazel-hint: remote cache unreachable, building locally (results are correct, the build is slower). Remedy: none needed; run 'make bazel-doctor' to check the cache.

<!-- errors.yaml:E11 -->
!! bazel-hint: the remote cache REJECTED our credentials (HTTP 401/403 or expired), so the shared cache is not being used. Remedy: refresh the credential ('make bazel-doctor' names which one); builds stay correct but slow.

`make bazel-doctor` check 2 probes the cache and names which of the two applies.
