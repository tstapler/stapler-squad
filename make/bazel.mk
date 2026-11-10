# Bazel conversion skeleton. `make` never needs Bazel: BUILD_SYSTEM defaults to `make` and every
# Bazel-dependent gate joins `ready` only when BUILD_SYSTEM=bazel (ADR-003, plan Story 1.5).
# Gates use: $(if $(filter bazel,$(BUILD_SYSTEM)),<gate>,)  (Epics 2/7/8 extend this file).

BUILD_SYSTEM ?= make
ifeq ($(filter $(BUILD_SYSTEM),make bazel),)
$(error BUILD_SYSTEM must be make or bazel)
endif

# BAZEL runs Bazel through tools/bazel (which needs BAZEL_REAL). tools/bazel itself adds --config=agent for
# non-TTY callers and never for `query`, so BAZEL_FLAGS stays empty by default; set it to add flags to every wrapper.
BAZEL ?= BAZEL_REAL=$$(command -v bazelisk || command -v bazel) tools/bazel
BAZEL_FLAGS ?=

# wrap: the banner line is the exact command, so an agent sees which system ran and can copy it.
define wrap
@echo "[build-system=bazel] bazel $(1)"
$(BAZEL) $(1)
endef

.PHONY: nogo-twins-check nogo-seed-check
.PHONY: gazelle gazelle-check buildifier-check sync gen-freshness go-build go-vet go-test ready-bazel-guards bazel-doctor gen-gate gen-gate-strict

bazel-doctor: ## Nine host/worktree Bazel health checks (version, cache, disk, slots, memory, BUILD freshness); exit 0 unless one fails
	bash tools/bzl/doctor.sh

gazelle: ## Regenerate BUILD files with Gazelle (needs Bazel), then refresh the .gitattributes collapse rules
	$(call wrap,run //:gazelle)
	@bash tools/bzl/gen-gitattributes.sh

nogo-twins-check: ## Fail when tools/lint analyzers, the nogo twin list and cmd/linter disagree (plain go; no Bazel)
	go run ./tools/bzl/nogotwins

nogo-seed-check: ## Build the manual seeded-violation targets; every analyzer must fail its own seed under nogo (needs Bazel)
	BAZEL_REAL=$$(command -v bazelisk || command -v bazel) go run ./tools/bzl/nogoseed

gazelle-check: ## Fail if BUILD files or MODULE.bazel are out of date (bazel run //:gazelle -- -mode=diff, then bazel mod tidy diff); needs Bazel, skipped under BUILD_SYSTEM=make
ifeq ($(BUILD_SYSTEM),make)
	@echo "skipped: BUILD_SYSTEM=make"
else
	@echo "gate: gazelle-check"
	$(call wrap,run //:gazelle -- -mode=diff)
	@echo "[build-system=bazel] bazel mod tidy (MODULE.bazel must not change)"
	@before=$$(sha256sum MODULE.bazel); $(BAZEL) mod tidy; \
	[ "$$before" = "$$(sha256sum MODULE.bazel)" ] || { echo "MODULE.bazel was not tidy: 'bazel mod tidy' rewrote it; review and commit the change"; exit 1; }
endif

buildifier-check: ## Fail if a BUILD or .bzl file needs buildifier fixes or has lint warnings (buildifier -mode=check -lint=warn, the bazel.yml command); skipped under BUILD_SYSTEM=make
ifeq ($(BUILD_SYSTEM),make)
	@echo "skipped: BUILD_SYSTEM=make"
else
	@echo "gate: buildifier-check"
	@out=$$( (command -v buildifier >/dev/null 2>&1 && buildifier -mode=check -lint=warn -r . || go run github.com/bazel-contrib/buildtools/v10/buildifier@v10.1.0 -mode=check -lint=warn -r .) 2>&1 ); \
	if [ -n "$$out" ]; then printf '%s\n' "$$out" | head -40; exit 1; fi
endif

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
	@echo "gate: gen-freshness"
	@bash tools/bzl/check-gen-fresh-cheap.sh || { echo "run: make sync"; exit 1; }
	@bash tools/bzl/check-gen-fresh.sh
endif

# Cheap gen-freshness (Task 8.2d), listed first among a wrapper's prerequisites. gen-gate is the note
# for Bazel-run recipes (Bazel ignores gen/); gen-gate-strict fails for recipes that run native Go tools.
gen-gate:
	@bash tools/bzl/check-gen-fresh-cheap.sh --note

gen-gate-strict:
	@bash tools/bzl/check-gen-fresh-cheap.sh --strict

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
	bash tools/bzl/check-pkg-mapping.sh
	bash tools/bzl/check-wrappers.sh

# ---- build and test wrappers (Epic 8). Under BUILD_SYSTEM=make the Makefile's legacy recipes apply; under
# BUILD_SYSTEM=bazel the rules below replace them. Descriptions live on the legacy rules in Makefile (help
# greps the text of both files) and each names its Bazel command; tools/bzl/check-make-help.sh enforces it.

# PKG/RUN/TAGS/TEST_TAGS/RACE/FORCE mapping (Task 8.2e; table: tools/bzl/testdata/pkg-mapping.tsv).
BZL_VALID_TAGS := embed_tmux embed_tymux sinkguard gitoracle integration race release
bzl_uniq = $(if $(1),$(firstword $(1)) $(call bzl_uniq,$(filter-out $(firstword $(1)),$(1))))
bzl_tag_check = $(foreach t,$(1),$(if $(filter $(t),$(BZL_VALID_TAGS)),,$(error unknown TAGS value '$(t)'; valid: $(BZL_VALID_TAGS))))
bzl_configs = $(call bzl_tag_check,$(1))$(foreach t,$(call bzl_uniq,$(1)),--config=$(t))
bzl_labels = $(foreach p,$(1),$(if $(filter ./... ...,$(p)),//...,$(if $(filter %/...,$(p)),//$(patsubst ./%,%,$(p)),$(if $(filter .,$(p)),//:all,//$(patsubst ./%,%,$(p)):all))))
bzl_test_flags = $(strip $(call bzl_configs,$(TAGS) $(1) $(if $(filter 1,$(RACE)),race)) $(if $(TEST_TAGS),--test_tag_filters=$(TEST_TAGS)) $(if $(RUN),--test_filter=$(RUN)) $(if $(FORCE),--nocache_test_results))

# bzl_test: $(1) extra TAGS, $(2) package words, $(3) extra flags (before the labels).
define bzl_test
@echo "[build-system=bazel] bazel test $(strip $(BAZEL_FLAGS) $(call bzl_test_flags,$(1)) $(3) $(call bzl_labels,$(2)))"
@bash tools/bzl/check-build-fresh-cheap.sh --warn | grep -v '^BUILD: fresh$$' || true
@if bash tools/bzl/pkg-has-tests.sh $(2); then bash tools/bzl/run-bazel-test.sh $(strip $(BAZEL_FLAGS) $(call bzl_test_flags,$(1)) $(3) $(call bzl_labels,$(2))); fi
endef

# bzl_build: $(1) flags, $(2) target.
define bzl_build
@echo "[build-system=bazel] bazel build $(strip $(BAZEL_FLAGS) $(1) $(2))"
@bash tools/bzl/check-build-fresh-cheap.sh --warn | grep -v '^BUILD: fresh$$' || true
$(BAZEL) build $(strip $(BAZEL_FLAGS) $(1) $(2))
endef

# bzl_publish: copy (never symlink) a bazel-bin output to the path the make recipe produces.
define bzl_publish
@rm -f $(2) && install -m 0755 $(1) $(2)
@echo "[build-system=bazel] copied $(1) -> $(2)"
endef

ifeq ($(BUILD_SYSTEM),bazel)

.PHONY: build stapler-squad build-embedded build-embedded-tymux build-tmux web-build proto-gen ent-gen install-service \
        test test-race test-integration test-oracle test-delivery-guards test-affected

build: stapler-squad
	@echo "[build-system=bazel] build is stapler-squad: bazel build //:stapler-squad, copied to ./stapler-squad"

stapler-squad: gen-gate
	$(call bzl_build,,//:stapler-squad)
	$(call bzl_publish,bazel-bin/stapler-squad,stapler-squad)

build-embedded: gen-gate
	$(call bzl_build,--config=embed_tmux,//:stapler-squad)
	$(call bzl_publish,bazel-bin/stapler-squad,stapler-squad)

build-embedded-tymux: gen-gate
	$(call bzl_build,--config=embed_tymux,//:stapler-squad)
	$(call bzl_publish,bazel-bin/stapler-squad,stapler-squad)

build-tmux:
	$(call bzl_build,,//tools/bzl/tmux:tmux_bin)
	@mkdir -p bin
	$(call bzl_publish,bazel-bin/tools/bzl/tmux/tmux,bin/tmux)

web-build:
	$(call bzl_build,,//server/web:dist_staged)
	@rm -rf server/web/dist && cp -r bazel-bin/server/web/dist server/web/dist && chmod -R u+w server/web/dist
	@echo "[build-system=bazel] copied bazel-bin/server/web/dist -> server/web/dist"

proto-gen ent-gen: sync
	@echo "[build-system=bazel] $@ is an alias of 'make sync' (bazel run //:sync-gen)"

test: gen-gate
	$(call bzl_test,,$(or $(PKG),./...),--test_arg=-test.short)

test-race: gen-gate
	$(call bzl_test,race,$(or $(PKG),./...),--test_arg=-test.short)

test-integration: gen-gate
	$(call bzl_test,integration race,$(or $(PKG),./...))

BZL_ORACLE_PKGS := ./testutil/gitoracle/... ./session/git/backend/gogit/... ./session/gitwiring/... ./session/git/native/gitconfig/... ./session/git/native/capability/...
test-oracle: gen-gate
	$(call bzl_test,gitoracle race,$(BZL_ORACLE_PKGS))

# One bazel test per directory with sinkguard tests, filtered to just those Test funcs (as the make recipe does).
test-delivery-guards: gen-gate
	@echo "[build-system=bazel] bazel test --config=sinkguard --test_filter=^(<sinkguard Test funcs>)$$ //<dir>:all, once per directory with a sinkguard test file"
	@set -e; for d in $$(grep -rl --include='*_test.go' '^//go:build sinkguard' server session cmd pkg 2>/dev/null | xargs -r -n1 dirname | sort -u); do \
		names=$$(grep -h -oE '^func (Test[A-Za-z0-9_]+)' $$(grep -l '^//go:build sinkguard' $$d/*_test.go) | sed 's/^func //' | paste -sd'|' -); \
		[ -n "$$names" ] || continue; \
		bash tools/bzl/run-bazel-test.sh --config=sinkguard "--test_filter=^($$names)$$" //$$d:all; \
	done

# install-service (Task 9.1a): build the release binary and the verifier in ONE invocation (one output tree, so
# bazel-bin holds both), copy it to the stable ./stapler-squad (atomic rename; ExecStart never points into bazel-*),
# sign the copy on macOS, then install-service.sh runs verifybin before it stops the old unit. Owner-run only.
BZL_RELEASE_TARGETS := //:stapler-squad //tools/bzl/verifybin //session/tmux/embed:tmux_staged //session/tymux/embed:tymuxd_staged
install-service: install-service-preflight backup-binary gen-gate install-hooks
	$(call bzl_build,--config=release,$(BZL_RELEASE_TARGETS))
	@cp bazel-bin/stapler-squad ./stapler-squad.new && chmod 0755 ./stapler-squad.new && mv -f ./stapler-squad.new ./stapler-squad
	@echo "[build-system=bazel] copied bazel-bin/stapler-squad -> ./stapler-squad"
ifeq ($(UNAME_S),Darwin)
	@$(MAKE) _codesign-binary BIN=$(CURDIR)/stapler-squad
endif
	@SSQ_VERIFY_BIN_TOOL="$(CURDIR)/bazel-bin/tools/bzl/verifybin/verifybin_/verifybin" \
		SSQ_VERIFY_TMUX="$(CURDIR)/bazel-bin/session/tmux/embed/tmux" \
		SSQ_VERIFY_TYMUXD="$(CURDIR)/bazel-bin/session/tymux/embed/tymuxd" \
		$(if $(filter Darwin,$(UNAME_S)),SSQ_VERIFY_DR_DOC="$(CURDIR)/docs/how-to/macos-codesigning.md") \
		STAPLER_SQUAD_BIN="$(CURDIR)/stapler-squad" ./scripts/install-service.sh $(if $(NO_PROFILE),--no-profile) $(if $(PROFILE_PORT),--profile-port $(PROFILE_PORT))

test-affected: gen-gate
	@echo "[build-system=bazel] bazel test <labels tools/ci/affected-targets selects vs $(or $(BASE),origin/main)>"
	$(call bzl_build,,//tools/ci/affected-targets)
	@set -e; sel="$$(BAZEL_REAL=$$(command -v bazelisk || command -v bazel) bazel-bin/tools/ci/affected-targets/affected-targets_/affected-targets -bazel tools/bazel -base $(or $(BASE),origin/main))"; \
	labels=$$(printf '%s\n' "$$sel" | grep '^//' || true); \
	if [ -z "$$labels" ]; then echo "No targets affected by changes vs $(or $(BASE),origin/main)"; \
	else echo "Affected labels:"; echo "$$labels"; \
		bash tools/bzl/run-bazel-test.sh --test_arg=-test.short $$labels; fi

endif
