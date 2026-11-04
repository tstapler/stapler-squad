# Bazel conversion skeleton. `make` never needs Bazel: BUILD_SYSTEM defaults to `make` and every
# Bazel-dependent gate joins `ready` only when BUILD_SYSTEM=bazel (ADR-003, plan Story 1.5).
# Gates use: $(if $(filter bazel,$(BUILD_SYSTEM)),<gate>,)  (Epics 2/7/8 extend this file).

BUILD_SYSTEM ?= make
ifeq ($(filter $(BUILD_SYSTEM),make bazel),)
$(error BUILD_SYSTEM must be make or bazel)
endif

# Wrapper stub: runs a Bazel command through tools/bazel (Story 8.1 adds BAZEL_FLAGS and wrappers).
define wrap
BAZEL_REAL=$$(command -v bazelisk || command -v bazel) tools/bazel $(1)
endef

.PHONY: gazelle gazelle-check sync ready-bazel-guards bazel-doctor

bazel-doctor: ## Nine host/worktree Bazel health checks (version, cache, disk, slots, memory, BUILD freshness); exit 0 unless one fails
	bash tools/bzl/doctor.sh

gazelle: ## Regenerate BUILD files with Gazelle (needs Bazel)
	$(call wrap,run //:gazelle)

gazelle-check: ## Fail if BUILD files are out of date with the Go sources (Gazelle -mode=diff; needs Bazel)
	$(call wrap,run //:gazelle -- -mode=diff)

sync: ## Generate code plain go/gopls need (BUILD_SYSTEM=make: proto-gen ent-gen; Bazel branch lands in Epic 2)
ifeq ($(BUILD_SYSTEM),make)
	$(MAKE) proto-gen ent-gen
else
	@echo "sync: the BUILD_SYSTEM=bazel branch arrives with Tasks 2.4a-2.4d; use BUILD_SYSTEM=make" >&2; exit 1
endif

# check-bazelrc.sh fails closed without bazel; under BUILD_SYSTEM=make with none on PATH its
# bazel-backed probes (flag existence via `bazel help`, `bazel mod graph`) are skipped, loudly.
BZL_NO_BAZEL_ENV = $(if $(shell command -v bazel >/dev/null 2>&1 || command -v bazelisk >/dev/null 2>&1 || echo none),$(if $(filter make,$(BUILD_SYSTEM)),BZL_CHECK_SKIP_HELP=1 BZL_CHECK_SKIP_MOD_GRAPH=1,),)

ready-bazel-guards: ## Bazel repo guardrails: .bazelignore, BUILD paths, .bazelrc, configs, no force-push (+ their tests); need only sh and git
	bash tools/bzl/check-bazelignore.sh
	bash tools/bzl/check-build-paths.sh
	@$(if $(BZL_NO_BAZEL_ENV),echo "skipped: BUILD_SYSTEM=make (check-bazelrc.sh flag and mod-graph probes; no bazel on PATH)",:)
	$(BZL_NO_BAZEL_ENV) bash tools/bzl/check-bazelrc.sh
	bash tools/bzl/check-bazelrc_test.sh
	bash tools/bzl/check-configs.sh
	bash tools/bzl/check-no-force-push.sh
	bash tools/bzl/translate-errors_test.sh
	bash tools/bzl/translate-errors_test.sh --negative
	bash tools/bzl/check-build-fresh-cheap_test.sh
	bash tools/bzl/doctor_test.sh
	bash tools/bzl/check-make-help.sh
	bash tools/bzl/check-no-force-push_test.sh
	bash scripts/hooks/pre-push-no-force_test.sh
