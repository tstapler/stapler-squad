"""e2e_test: a Playwright spec group whose declared inputs are its observed slice (ADR-009, Task 12.2a)."""

load("@rules_shell//shell:sh_test.bzl", "sh_test")

def e2e_test(name, specs, go_pkgs = [], web_files = [], tags = [], **kwargs):
    """One Playwright invocation over `specs`; the slice (go_pkgs, web_files) is the cache key, not the binary.

    Args:
      name: target name (the spec group).
      specs: spec file names relative to tests/e2e.
      go_pkgs: go_library labels the specs' server run executed (observed manifest).
      web_files: web source labels the specs' browser run loaded (observed manifest).
      tags: extra tags, e.g. no-cache for quarantined specs.
      **kwargs: forwarded to the test rule.
    """
    sh_test(
        name = name,
        srcs = ["//tools/bzl/e2erun"],
        args = ["-target=" + name] + ["-spec=" + s for s in specs],
        data = specs + go_pkgs + web_files + [":global_inputs"],
        tags = ["e2e", "block-network"] + tags,
        **kwargs
    )
