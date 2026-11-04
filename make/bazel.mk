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

.PHONY: gazelle gazelle-check sync gen-freshness go-build go-vet go-test ready-bazel-guards bazel-doctor

bazel-doctor: ## Nine host/worktree Bazel health checks (version, cache, disk, slots, memory, BUILD freshness); exit 0 unless one fails
	bash tools/bzl/doctor.sh

gazelle: ## Regenerate BUILD files with Gazelle (needs Bazel), then refresh the .gitattributes collapse rules
	$(call wrap,run //:gazelle)
	@bash tools/bzl/gen-gitattributes.sh

gazelle-check: ## Fail if BUILD files are out of date with the Go sources (Gazelle -mode=diff; needs Bazel)
	$(call wrap,run //:gazelle -- -mode=diff)

sync: ## Generate code plain go/gopls need into gen/ and session/ent (BUILD_SYSTEM=make: proto-gen ent-gen; bazel: bazel run //:sync-gen)
ifeq ($(BUILD_SYSTEM),make)
	$(MAKE) proto-gen ent-gen
	@bash tools/bzl/check-gen-fresh-cheap.sh --write
else
	$(call wrap,run --ui_event_filters=-info --noshow_progress //:sync-gen)
endif

gen-freshness: ## Fail (exit 1, "run: make sync") if gen/ and session/ent are stale; BUILD_SYSTEM=bazel only (skipped under make)
ifeq ($(BUILD_SYSTEM),make)
	@echo "skipped: BUILD_SYSTEM=make"
else
	@bash tools/bzl/check-gen-fresh-cheap.sh || { echo "run: make sync"; exit 1; }
	@bash tools/bzl/check-gen-fresh.sh
endif

# Native-go shims (Task 2.4e): raw `go build` after a pull is unprotected by design; these run `make sync`
# when the inputs stamp is stale, then the native tool, translating a failure when translate-errors.sh exists.
NATIVE_GO_BANNER = [native go - not authoritative; Bazel is the build of record]
define native_go
@echo "$(NATIVE_GO_BANNER)"
@case "$$(bash tools/bzl/check-gen-fresh-cheap.sh --warn)" in *STALE*) $(MAKE) sync;; esac
@out=$$($(1) 2>&1); rc=$$?; \
 if [ $$rc != 0 ] && [ -f tools/bzl/translate-errors.sh ]; then printf '%s\n' "$$out" | bash tools/bzl/translate-errors.sh --source go; else printf '%s\n' "$$out"; fi; exit $$rc
endef

go-build: ## Native go build ./... with auto-sync of stale generated code (not authoritative; Bazel is)
	$(call native_go,go build ./...)

go-vet: ## Native go vet ./... with auto-sync of stale generated code (not authoritative; Bazel is)
	$(call native_go,go vet ./...)

go-test: ## Native go test with auto-sync: make go-test PKG=./config RUN=TestFoo (not authoritative; Bazel is)
	$(call native_go,go test $(or $(PKG),./...) $(if $(RUN),-run '$(RUN)',))

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
	bash tools/bzl/gen-gitattributes.sh --check
	bash tools/bzl/check-gen-fresh-cheap_test.sh
	bash tools/bzl/check-no-force-push.sh
	bash tools/bzl/translate-errors_test.sh
	bash tools/bzl/translate-errors_test.sh --negative
	bash tools/bzl/check-build-fresh-cheap_test.sh
	bash tools/bzl/doctor_test.sh
	bash tools/bzl/check-make-help.sh
	bash tools/bzl/check-no-force-push_test.sh
	bash scripts/hooks/pre-push-no-force_test.sh
