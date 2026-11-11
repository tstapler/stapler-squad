"""Unit tests for go_test_defaults (the table and cache policy of go_test.bzl)."""

load("@bazel_skylib//lib:unittest.bzl", "asserts", "unittest")
load(":go_test.bzl", "go_test_defaults")

def _size_and_shards_impl(ctx):
    env = unittest.begin(ctx)
    got = go_test_defaults("server/services", {})
    asserts.equals(env, "enormous", got["size"])
    asserts.equals(env, 4, got["shard_count"])
    asserts.equals(env, "small", go_test_defaults("server/services", {"size": "small"})["size"])
    asserts.equals(env, {}, go_test_defaults("config", {}))
    return unittest.end(env)

def _host_coupled_gets_external_impl(ctx):
    env = unittest.begin(ctx)
    asserts.equals(env, ["x", "external"], go_test_defaults("session/vc", {"tags": ["x"]})["tags"])
    asserts.equals(env, ["external"], go_test_defaults("session", {})["tags"])
    asserts.false(env, "tags" in go_test_defaults("executor", {}))
    asserts.false(env, "tags" in go_test_defaults("github", {}))
    return unittest.end(env)

def _caller_kwargs_not_mutated_impl(ctx):
    env = unittest.begin(ctx)
    tags = ["manual"]
    go_test_defaults("session", {"tags": tags})
    asserts.equals(env, ["manual"], tags)
    return unittest.end(env)

_size_and_shards_test = unittest.make(_size_and_shards_impl)
_host_coupled_test = unittest.make(_host_coupled_gets_external_impl)
_not_mutated_test = unittest.make(_caller_kwargs_not_mutated_impl)

def go_test_bzl_test_suite(name):
    unittest.suite(name, _size_and_shards_test, _host_coupled_test, _not_mutated_test)
