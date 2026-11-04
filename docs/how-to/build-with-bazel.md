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

Build, then copy the output to `~/.stapler-squad/manual-builds/manual-<N>/stapler-squad` and follow the manual-instance
recipe in `CLAUDE.md` (ports 62871-62880, private tmux socket). Do not `bazel run` the main binary against `:8543` and
do not overwrite `./stapler-squad`, the live unit's `ExecStart`. See also
[protect-live-service-from-bazel](protect-live-service-from-bazel.md).

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
