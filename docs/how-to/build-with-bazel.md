# Build with Bazel

Bazel is the build of record (provisional until cutover): the only path that produces shipped artifacts, release
binaries, parity evidence and the CI verdict. Native `go build`, `go vet`, `go test` and gopls are supported for
editor tooling, golangci-lint, quick local iteration and bisecting after `make sync`; a native `go test` result is never
authoritative.

Every command below is a proposal until `tools/bzl/verified-commands.txt` lists it with its run log; the
`<!-- proposal -->` marker is what `tools/bzl/check-skill-proposals.sh` looks for.

## Run Bazel the supported way

Use `tools/bazel`, not a bare `bazel`. Bazelisk runs it automatically; a direct call needs `BAZEL_REAL` set. It takes
one of two host-wide build slots (`BAZEL_WRAPPER_SLOTS`), runs Bazel in a memory-capped systemd scope and, for agents,
adds `--config=agent`. See [first-build-in-a-fresh-worktree](first-build-in-a-fresh-worktree.md) for the queue messages.

## Build the binary

<!-- proposal -->
```bash
bazel build //:stapler-squad
```

## Test one package or one test

<!-- proposal -->
```bash
bazel test //session:session_test --test_output=errors
bazel test //session:session_test --test_filter=TestFoo --test_output=errors
```

`--test_filter` reaches Go as an unanchored regular expression, like `go test -run`. Add `--nocache_test_results` to
force a rerun (the old `-count=1`).

## Add a Go dependency (proposal - not spike-verified)

```bash
go get example.com/mod@vX.Y.Z
go mod tidy
make gazelle
bazel mod tidy
```

If `use_repo` is missing the failure is `no such package '@@gazelle++go_deps+...'`; see
[troubleshoot-bazel](troubleshoot-bazel.md#state-2).

## Regenerate BUILD files

After changing imports or adding or removing a `.go` file:

<!-- proposal -->
```bash
make gazelle
```

Commit the BUILD diff. Never hand-edit a Gazelle-managed `BUILD.bazel`; the hand-written ones are listed in
`tools/bzl/hand-build-files.txt`. `make gazelle-check` fails when BUILD files are stale. <!-- proposal -->

## Go tooling and gopls after cutover

Bazel needs no generated code; plain `go` and gopls do. Run `make sync` to materialise `gen/` and `session/ent`.
<!-- proposal -->
This inverts the old `make proto-gen ent-gen` precondition. `make go-build`, `make go-vet` and `make go-test` sync
first when the inputs changed. <!-- proposal -->

Post-cutover status, verbatim: Bazel is the build of record; native `go build`/`go vet`/`go test` and gopls are
supported for editor tooling, golangci-lint, quick local iteration and bisecting after `make sync`, and a native
`go test` result is never authoritative. `GOPACKAGESDRIVER` (gopackagesdriver) is optional and unsupported by default.

## Run a manual instance, never against the live service

Use the isolated manual instance (parity row P9): `bazel run //tools/bzl/manual-instance -- --dry-run` prints the exact
commands and runs nothing, and without `--dry-run` it builds, copies the binary to
`~/.stapler-squad/manual-builds/manual-1/stapler-squad` and starts it on ports 62871/62872 with
`STAPLER_SQUAD_INSTANCE=claude-manual-test` and the private tmux socket `ssq-manual-1` (`--stop` stops it; source:
`tools/bzl/manual-instance/main.go`, a Go tool rather than the `scripts/bazel-manual-instance.sh` the plan named). The
manual recipe in `CLAUDE.md` is the same thing by hand. Do not `bazel run` the main binary against `:8543` and do not
overwrite `./stapler-squad`, the live unit's `ExecStart`. See also
[protect-live-service-from-bazel](protect-live-service-from-bazel.md).

## Install the service from the Bazel binary (owner-run, never by an agent)

`make install-service` replaces the live `:8543` service. Agents must not run it, nor `scripts/install-service.sh`, to
test a change: use the manual instance above, or `make -n install-service` and
`sh scripts/install-service.sh --dry-run` (Linux; prints the unit it would write and touches nothing).

Order, no shortcuts:

1. Parity gate P9 (the manual instance above) passes the smoke list.
2. You record the attestation in the PR (the owner decision; nothing automates it).
3. You run `BUILD_SYSTEM=bazel make install-service` from a clean checkout of the commit you mean to ship.

Under `BUILD_SYSTEM=bazel` the target runs the preflight and `backup-binary` (as under make), then
`bazel build --config=release //:stapler-squad //tools/bzl/verifybin ...`, copies the result to `./stapler-squad`
(never `bazel-bin`; `ExecStart` never points into `bazel-*`), signs that copy on macOS, and runs
`scripts/install-service.sh`. Before the script stops the old unit it runs `tools/bzl/verifybin`, which aborts the
install (old service untouched) unless the binary is a regular file outside any Bazel output tree, `version` prints a
stamped version (not `dev`), the staged tmux and tymuxd outputs are embedded byte for byte (their sha256 is printed;
the pins themselves are checked by `tools/ci/check-tymux-pin.sh`), and, on macOS, `codesign -d -r-` equals the
designated requirement recorded in [macos-codesigning](macos-codesigning.md). Under the default
`BUILD_SYSTEM=make` none of this applies and the recipe is unchanged.

The generated unit sets `OOMScoreAdjust=-900` and `OOMPolicy=continue`; an operator drop-in still overrides the score,
see [protect-live-service-from-bazel](protect-live-service-from-bazel.md).

## Merge conflicts in `MODULE.bazel.lock`

Register the lock merge driver once per clone, and again after every fresh `git clone` (git config is not versioned):

<!-- proposal -->
```bash
make setup-git
```

It sets `merge.bazel-lockfile-merge.driver` to `scripts/git-merge-bazel-lock.sh %O %A %B` (needs `jq` 1.5 or newer),
which `.gitattributes` applies to `MODULE.bazel.lock`. `scripts/git-merge-bazel-lock.sh --check` and `make bazel-doctor`
check 9 report a missing registration. CI regenerates the lock and fails on a diff.

## Something looks wrong

Run `make bazel-doctor` first. <!-- proposal --> Every error state has an entry in
[troubleshoot-bazel](troubleshoot-bazel.md).
