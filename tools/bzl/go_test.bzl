"""go_test with per-package size, shard and cache-policy defaults; Gazelle emits it via `# gazelle:map_kind` (root BUILD.bazel)."""

load("@rules_go//go:def.bzl", _go_test = "go_test")

# package -> (size, shard_count). One table so server/services (20m under make) is not cut off by Bazel's default timeout.
_SIZES = {
    "server/services": ("enormous", 4),
    "session": ("large", 2),
}

# package -> SandboxClasses (Epic 0 census S8/S9 and test-exclusions.yaml) that make a result depend on the host.
# A ratchet: entries leave as Story 4.10 sweeps declare each test's inputs; none may be added without a census row.
_HOST_COUPLED = {
    "server/services": ["exec-host-binary"],
    "session": ["exec-host-binary", "home"],
    "session/vc": ["home"],
}

_EXTERNAL = "external"  # tag-reason: host-coupled result must rerun, never cache (pre-mortem #3) (#1423)

def go_test_defaults(package, kwargs):
    """Returns kwargs with the table's size, shard_count and cache-policy tags applied.

    Args:
      package: the Bazel package name, the key into the size and host-coupling tables.
      kwargs: the go_test keyword arguments to adjust.

    Returns:
      A copy of kwargs with size, shard_count and tags filled in.
    """
    if kwargs.get("flaky"):
        fail("the flaky attribute is banned (Task 0.2g): fix the test instead of retrying it")
    out = dict(kwargs)
    size, shards = _SIZES.get(package, (None, None))
    if size and "size" not in out:
        out["size"] = size
    if shards and "shard_count" not in out:
        out["shard_count"] = shards
    tags = list(out.get("tags", []))
    if _HOST_COUPLED.get(package) and _EXTERNAL not in tags:
        tags.append(_EXTERNAL)
    if tags:
        out["tags"] = tags
    return out

def go_test(name, **kwargs):
    _go_test(name = name, **go_test_defaults(native.package_name(), kwargs))
