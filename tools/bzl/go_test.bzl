"""go_test with per-package size and shard defaults; Gazelle emits it via `# gazelle:map_kind` (root BUILD.bazel)."""

load("@rules_go//go:def.bzl", _go_test = "go_test")

# package -> (size, shard_count). Epic 4 adds sandbox tags and timeouts here, in one place.
_DEFAULTS = {
    "server/services": ("enormous", 4),
    "session": ("large", 2),
}

def go_test(name, **kwargs):
    size, shards = _DEFAULTS.get(native.package_name(), (None, None))
    if size and "size" not in kwargs:
        kwargs["size"] = size
    if shards and "shard_count" not in kwargs:
        kwargs["shard_count"] = shards
    _go_test(name = name, **kwargs)
