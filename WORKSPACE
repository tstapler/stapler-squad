workspace(name = "stapler_squad")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

# rules_foreign_cc wraps autoconf/make builds so Bazel can cache C artifacts
http_archive(
    name = "rules_foreign_cc",
    sha256 = "476303bd0f1b31bd2c5c0e5af4bff6a3b8b3ac61c89b9399d70f7d3afcbade01",
    strip_prefix = "rules_foreign_cc-0.10.1",
    url = "https://github.com/bazelbuild/rules_foreign_cc/releases/download/0.10.1/rules_foreign_cc-0.10.1.tar.gz",
)

load("@rules_foreign_cc//foreign_cc:repositories.bzl", "rules_foreign_cc_dependencies")

rules_foreign_cc_dependencies()
