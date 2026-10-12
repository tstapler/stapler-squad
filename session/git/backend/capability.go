package backend

import (
	"context"
	"errors"
)

// CapabilityKind is a repository feature the in-process backend cannot reproduce faithfully
// (ADR-006). It is a closed enum, safe as a metric label.
type CapabilityKind string

const (
	CapHooks            CapabilityKind = "hooks"
	CapGPGSign          CapabilityKind = "gpgsign"
	CapLFS              CapabilityKind = "lfs"
	CapUnsupportedIndex CapabilityKind = "unsupported_index"
	CapSparse           CapabilityKind = "sparse"
	CapShallow          CapabilityKind = "shallow"
	CapSubmodule        CapabilityKind = "submodule"
	CapSSHProxy         CapabilityKind = "ssh_proxy"
	CapLocalTransport   CapabilityKind = "local_transport"
)

var capabilityReasons = map[CapabilityKind]FallbackReason{
	CapHooks: ReasonCapabilityHooks, CapGPGSign: ReasonCapabilityGPGSign, CapLFS: ReasonCapabilityLFS,
	CapUnsupportedIndex: ReasonCapabilityUnsupportedIdx, CapSparse: ReasonCapabilitySparse,
	CapShallow: ReasonCapabilityShallow, CapSubmodule: ReasonCapabilitySubmodule,
	CapSSHProxy: ReasonCapabilitySSHProxy, CapLocalTransport: ReasonCapabilityLocalTransport,
}

// Reason is the FallbackReason a detected capability routes with.
func (k CapabilityKind) Reason() FallbackReason {
	if r, ok := capabilityReasons[k]; ok {
		return r
	}
	return ReasonCapabilityDetectError // an unknown kind from a probe must still route to the CLI
}

// Capability is one detected feature. Detail is diagnostic only ("pre-commit" for a hook) and
// is never a metric label.
type Capability struct {
	Kind   CapabilityKind
	Detail string
}

// worktreeContentOps read or write working-tree file content or the index, so a clean/smudge
// filter, a sparse checkout, a split index or a submodule changes their result.
var worktreeContentOps = func() map[OperationName]bool {
	m := map[OperationName]bool{
		OpAdd: true, OpCommit: true, OpIsDirty: true, OpStatus: true, OpListUntracked: true,
		OpDiff: true, OpDiffNumstat: true,
	}
	for op := range worktreeFileWriters {
		m[op] = true
	}
	return m
}()

// graphOps walk history, which a shallow repository truncates.
var graphOps = map[OperationName]bool{
	OpMergeBase: true, OpCountCommits: true, OpLog: true, OpFetch: true, OpPull: true, OpPush: true,
}

// networkOps talk to a remote. Clone has no repository to inspect yet; its transport checks
// belong to the network cohort (plan Story 3.2.1), which sees the URL.
var networkOps = map[OperationName]bool{OpListRemote: true, OpFetch: true, OpPull: true, OpPush: true}

// signingOps can create a signed commit or push certificate.
var signingOps = map[OperationName]bool{OpCommit: true, OpPull: true, OpPush: true}

// RelevantCapabilities lists the capabilities that change what the in-process backend would do
// for op (plan Story 1.3.4: hooks for every mutating operation, signing for commit-creating
// ones, content facts for operations touching files or the index, and so on). Operations with
// none, such as the refs-cohort reads, never consult the probe.
func RelevantCapabilities(op OperationName) []CapabilityKind {
	if op == OpClone {
		return nil // no repository exists yet to inspect
	}
	var kinds []CapabilityKind
	if op.Mutating() {
		kinds = append(kinds, CapHooks)
	}
	if signingOps[op] {
		kinds = append(kinds, CapGPGSign)
	}
	if worktreeContentOps[op] {
		kinds = append(kinds, CapLFS, CapUnsupportedIndex, CapSparse, CapSubmodule)
	}
	if op == OpFetch {
		kinds = append(kinds, CapSubmodule) // fetch recurses into submodules on demand
	}
	if graphOps[op] {
		kinds = append(kinds, CapShallow)
	}
	if networkOps[op] {
		kinds = append(kinds, CapLocalTransport, CapSSHProxy)
	}
	return kinds
}

// CapabilityProbe detects the capabilities of the repository at loc that matter for op. It must
// compute only RelevantCapabilities(op), never cache hooks or signing, and return a
// *DetectError (or any error) when it cannot decide, which routes the call to the CLI.
type CapabilityProbe interface {
	Capabilities(ctx context.Context, loc Local, op OperationName) ([]Capability, error)
}

// DetectSub is the closed set of reasons a capability probe failed closed. It labels
// git_capability_detect_total{outcome="detect_error"} so a regression names itself.
type DetectSub string

const (
	DetectParse             DetectSub = "parse_error"
	DetectUnreadable        DetectSub = "unreadable_file"
	DetectTooLarge          DetectSub = "file_too_large"
	DetectIncludeDepth      DetectSub = "include_depth"
	DetectUnknownCondition  DetectSub = "unknown_condition"
	DetectUnsupportedGlob   DetectSub = "unsupported_glob"
	DetectRemoteURLInclude  DetectSub = "remote_url_in_include"
	DetectBareRemoteURL     DetectSub = "bare_remote_url"
	DetectGitdirSymlink     DetectSub = "gitdir_symlink_ambiguous"
	DetectUnsupportedInc    DetectSub = "unsupported_include_path"
	DetectBadInclude        DetectSub = "bad_include"
	DetectEnvOverride       DetectSub = "env_override"
	DetectHomeUnknown       DetectSub = "home_unknown"
	DetectCommonDir         DetectSub = "commondir"
	DetectCanceled          DetectSub = "canceled"
	DetectIO                DetectSub = "io_error"
	DetectConfigParameters  DetectSub = "config_parameters"
	DetectNoProbe           DetectSub = "no_probe"
	DetectPanic             DetectSub = "panic"
	DetectUnclassified      DetectSub = "unclassified"
	DetectUnsupportedIndex  DetectSub = "index_unreadable"
	DetectAttributesUnread  DetectSub = "attributes_unreadable"
	DetectHooksUnreadable   DetectSub = "hooks_unreadable"
	DetectSSHConfigUnreadab DetectSub = "ssh_config_unreadable"
)

// AllDetectSubs lists every DetectSub, for tests that keep label enums in sync.
func AllDetectSubs() []DetectSub {
	return []DetectSub{
		DetectParse, DetectUnreadable, DetectTooLarge, DetectIncludeDepth, DetectUnknownCondition,
		DetectUnsupportedGlob, DetectRemoteURLInclude, DetectBareRemoteURL, DetectGitdirSymlink,
		DetectUnsupportedInc, DetectBadInclude, DetectEnvOverride, DetectHomeUnknown, DetectCommonDir,
		DetectCanceled, DetectIO, DetectConfigParameters, DetectNoProbe, DetectPanic, DetectUnclassified,
		DetectUnsupportedIndex, DetectAttributesUnread, DetectHooksUnreadable, DetectSSHConfigUnreadab,
	}
}

// DetectError is a probe failure with its closed sub-reason.
type DetectError struct {
	Sub DetectSub
	Err error
}

func (e *DetectError) Error() string {
	if e.Err == nil {
		return "git capability detection failed: " + string(e.Sub)
	}
	return "git capability detection failed: " + string(e.Sub) + ": " + e.Err.Error()
}

func (e *DetectError) Unwrap() error { return e.Err }

// detectSubOf classifies any probe error; an unknown one is DetectUnclassified so the label set
// stays closed.
func detectSubOf(err error) DetectSub {
	var de *DetectError
	if errors.As(err, &de) {
		for _, s := range AllDetectSubs() {
			if de.Sub == s {
				return s
			}
		}
	}
	return DetectUnclassified
}

// reasonsForCapabilities maps detected capabilities onto the reasons for op, ignoring any kind the
// operation does not depend on, so a probe that over-reports cannot misroute.
func reasonsForCapabilities(op OperationName, caps []Capability) []FallbackReason {
	relevant := map[CapabilityKind]bool{}
	for _, k := range RelevantCapabilities(op) {
		relevant[k] = true
	}
	var reasons []FallbackReason
	for _, c := range caps {
		if relevant[c.Kind] {
			reasons = append(reasons, c.Kind.Reason())
		}
	}
	return reasons
}
