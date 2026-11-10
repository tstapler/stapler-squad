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

## Measured on `ghostty-vt.wasm` (run [42724101812](https://github.com/tstapler/stapler-squad/actions/runs/42724101812))

Three fresh `ubuntu-latest` runners in sequence, `scripts/build-ghostty-wasm.sh --test`, no repository
cache in any phase (so downloads are paid every time):

| Phase | Build step | Remote cache hits |
|---|---|---|
| baseline (no remote cache) | 137 s | 0 |
| cold (proxy, empty) | 138 s | 0 |
| warm (proxy restored from cold) | 26 s | 2 of 2 cacheable actions |

Install of the proxy binary: 191 s on a cache miss (`cargo install`), 2 s on a binary-cache hit.
Cost of the cache itself: one pack of 124,756 bytes (9 blobs) plus a 110-byte manifest.

For comparison, the existing `actions/cache` entries for this job are ~206 MB each
(`ghostty-wasm-*`; `gh api repos/tstapler/stapler-squad/actions/caches?key=ghostty-wasm`), and warm
runs of the existing job took 9-21 s in the build step over the last 12 successful runs
(cold: 88-110 s). Most of those 206 MB are the repository cache (Zig and Ghostty downloads), which
the proxy does not replace.

**Conclusion for this target:** the proxy is roughly equal to the current disk cache on time and
~1650x smaller in the action-cache part, but it does not remove the need for the repository cache.
It is not worth switching `ghostty-wasm.yml` for. It becomes interesting for the full-graph job in
`bazel.yml` (PR #1517), where a disk-cache archive grows with every action; that is **not measured**.
Run the trial workflow (`bazel-cache-proxy-trial.yml`) against that job before deciding.
