"""Negative tests for forbidden_deps.bzl: each seeded guard must fail analysis with the expected message."""

load("@bazel_skylib//lib:unittest.bzl", "analysistest", "asserts")

def _impl(ctx):
    env = analysistest.begin(ctx)
    asserts.expect_failure(env, ctx.attr.msg)
    return analysistest.end(env)

guard_fails_test = analysistest.make(_impl, expect_failure = True, attrs = {"msg": attr.string(mandatory = True)})
