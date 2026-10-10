package backend

// FallbackReason says why the Router sent a call to the CLI instead of the cohort's backend.
// It is a closed enum so it is safe as a metric label (plan glossary); never put free text in one.
type FallbackReason string

const (
	ReasonRemoteHost               FallbackReason = "remote_host"
	ReasonConfig                   FallbackReason = "config"
	ReasonAgentTool                FallbackReason = "agent_tool"
	ReasonCapabilityDetectError    FallbackReason = "capability_detect_error"
	ReasonCapabilityLocalTransport FallbackReason = "capability_local_transport"
	ReasonCapabilityHooks          FallbackReason = "capability_hooks"
	ReasonCapabilityGPGSign        FallbackReason = "capability_gpgsign"
	ReasonCapabilityLFS            FallbackReason = "capability_lfs"
	ReasonCapabilityUnsupportedIdx FallbackReason = "capability_unsupported_index"
	ReasonCapabilitySparse         FallbackReason = "capability_sparse"
	ReasonCapabilityShallow        FallbackReason = "capability_shallow"
	ReasonCapabilitySubmodule      FallbackReason = "capability_submodule"
	ReasonCapabilitySSHProxy       FallbackReason = "capability_ssh_proxy"
	ReasonLiveSession              FallbackReason = "live_session"
	ReasonUnsafeWorktreeWrite      FallbackReason = "unsafe_worktree_write"
	ReasonUnsupportedPullMode      FallbackReason = "unsupported_pull_mode"
	ReasonLockUnavailable          FallbackReason = "lock_unavailable"
	ReasonTornRead                 FallbackReason = "torn_read"
	ReasonObjectMissing            FallbackReason = "object_missing"
	ReasonObjectNotFound           FallbackReason = "object_not_found"
	ReasonDestructiveConfirm       FallbackReason = "destructive_confirm"
	ReasonError                    FallbackReason = "error"
)

// reasonPrecedence is the plan's evaluation order when several pre-call reasons apply (first
// match wins). The reasons a gogit call itself produces come last; they are never pre-call
// candidates but are ranked so FirstReason is total.
var reasonPrecedence = []FallbackReason{
	ReasonRemoteHost, ReasonConfig, ReasonAgentTool, ReasonCapabilityDetectError,
	ReasonCapabilityLocalTransport, ReasonCapabilityHooks, ReasonCapabilityGPGSign,
	ReasonCapabilityLFS, ReasonCapabilityUnsupportedIdx, ReasonCapabilitySparse,
	ReasonCapabilityShallow, ReasonCapabilitySubmodule, ReasonCapabilitySSHProxy,
	ReasonLiveSession, ReasonUnsafeWorktreeWrite, ReasonUnsupportedPullMode, ReasonLockUnavailable,
	ReasonTornRead, ReasonObjectMissing, ReasonObjectNotFound, ReasonDestructiveConfirm, ReasonError,
}

// ReasonPrecedence returns every FallbackReason in evaluation order.
func ReasonPrecedence() []FallbackReason {
	return append([]FallbackReason(nil), reasonPrecedence...)
}

// Known reports whether r is a member of the closed enum.
func (r FallbackReason) Known() bool { return r.rank() >= 0 }

func (r FallbackReason) rank() int {
	for i, p := range reasonPrecedence {
		if p == r {
			return i
		}
	}
	return -1
}

// FirstReason returns the reasons' winner under the precedence list, or "" for none. A reason
// outside the enum is ranked as ReasonError rather than dropped, so an unknown probe answer
// still routes to the CLI.
func FirstReason(reasons ...FallbackReason) FallbackReason {
	best, bestRank := FallbackReason(""), len(reasonPrecedence)+1
	for _, r := range reasons {
		rank := r.rank()
		if rank < 0 {
			r, rank = ReasonError, ReasonError.rank()
		}
		if rank < bestRank {
			best, bestRank = r, rank
		}
	}
	return best
}
