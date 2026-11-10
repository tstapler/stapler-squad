# Use the Bazel cache proxy in CI

The only remote cache this repo allows is the GitHub Actions cache. The
[`bazel-cache-proxy`](https://github.com/tstapler/bazel-cache-proxy) composite actions in
`.github/actions/bazel-cache-proxy/` run a loopback Bazel HTTP cache that persists its blobs there.

```yaml
- uses: ./.github/actions/bazel-cache-proxy/start
  with:
    prefix: my-job            # [A-Za-z0-9._-], distinct per workflow/job
- run: bazel build --remote_cache=http://127.0.0.1:9090/cache //...
- if: always()
  uses: ./.github/actions/bazel-cache-proxy/flush
```

`start` publishes on non-`pull_request` events only (PRs read the base branch's cache, which keeps
PR writes out of the 10 GB budget); pass `upload: 'true'` to override. Give concurrent jobs
(matrix legs) different prefixes: two jobs sharing one prefix overwrite each other's manifest.
Add `--experimental_remote_cache_eviction_retries=5` when using `--remote_download_toplevel`.

## Measured on `ghostty-vt.wasm` (run [42726789870](https://github.com/tstapler/stapler-squad/actions/runs/42726789870), proxy rev `54d4cf6`, one sample per phase)

Three fresh `ubuntu-latest` runners in sequence, `scripts/build-ghostty-wasm.sh --test`, no repository
cache in any phase (so downloads are paid every time):

| Phase | Build step | Remote cache hits |
|---|---|---|
| baseline (no remote cache) | 110 s | 0 |
| cold (proxy, empty) | 115 s | 0 |
| warm (proxy restored from cold) | 25 s | 2 of 2 cacheable actions |

Install of the proxy binary: 191 s on a cache miss (`cargo install`, from an earlier run of this trial), ~2 s on a binary-cache hit.
Cost of the cache itself: one pack of 124,765 bytes (9 blobs) plus a ~110-byte manifest. The same
baseline took 137 s in an earlier run of the trial: runner-to-runner variance is about 25 s, so
treat the cold-vs-baseline difference (5 s) as noise-level.

For comparison, the existing `actions/cache` entries for this job are ~206 MB each
(`ghostty-wasm-*`; `gh api repos/tstapler/stapler-squad/actions/caches?key=ghostty-wasm`), and warm
runs of the existing job took 9-21 s in the build step over the last 12 successful runs
(cold: 88-110 s). Most of those 206 MB are the repository cache (Zig and Ghostty downloads), which
the proxy does not replace.

**Conclusion for this target:** the proxy stores far less (125 KB vs a 206 MB entry that mixes the
disk and repository caches, so not like for like; the split was not measured) and its warm build
(25 s, no repository cache) is not faster than the existing job's 9-21 s with one. It does not
remove the need for the repository cache, so it is not worth switching `ghostty-wasm.yml` for. It becomes interesting for the full-graph job in
`bazel.yml` (PR #1517), where a disk-cache archive grows with every action; that is **not measured**.
Run the trial workflow (`bazel-cache-proxy-trial.yml`) against that job before deciding.

## Caveats before adopting it in `bazel.yml`

- `ghaction-github-runtime` exports the cache token to every later step of the job, so any build
  step could read or write cache entries. Keep it to jobs that build only trusted code.
- `#1532` routes cache writes through its `cache-main-writes` policy (main-only writes, pruned
  families). This action writes its own `<prefix>-pack-*` and `<prefix>-manifest-*` entries and its
  binary cache directly, so it needs reconciling with that policy once `#1532` lands.
- If the proxy fails to start the action only warns, and Bazel is still given the `--remote_cache`
  URL; Bazel's fallback to local execution in that case was not tested.
