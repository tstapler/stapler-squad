package session

import "fmt"

// KillOutcomeStatus enumerates the possible results of
// ConfirmKillExternalSession. This is a Go domain type, not a proto message
// -- the RPC handler maps it onto sessionv1.KillStatus.
type KillOutcomeStatus int

const (
	// KillOutcomeUnspecified is the zero value and is never returned by
	// KillExternalOriginalProcess.
	KillOutcomeUnspecified KillOutcomeStatus = iota
	// KillOutcomeKilled means the tmux session was killed successfully.
	KillOutcomeKilled
	// KillOutcomeAlreadyGone means IsAlive re-verification failed (PID reuse
	// or the process already exited) -- no signal was sent.
	KillOutcomeAlreadyGone
	// KillOutcomeFailed means the kill primitive itself (tmux kill-session)
	// failed. The original process is left SIGSTOP'd; it is never
	// auto-resumed on this path.
	KillOutcomeFailed
)

// KillOutcome is the domain-level result of KillExternalOriginalProcess.
type KillOutcome struct {
	Status KillOutcomeStatus
	Err    error
}

// AliveChecker is the subset of procinfo.ProcessInspector needed to
// re-verify a PID's identity immediately before killing it (scoped
// narrowly per the `interface-pollution-checklist` skill).
type AliveChecker interface {
	IsAlive(pid int32, expectedCreateTimeMs int64) bool
}

// KillExternalOriginalProcess re-verifies pid/createTimeMs are still the
// same process (guarding against PID reuse in the window between commit and
// this call) and, if so, resumes it (it was SIGSTOP'd at commit time -- a
// stopped process ignores signals sent by "tmux kill-session" until it's
// running again) and kills its tmux session via a throwaway
// InstanceTypeExternal Instance's KillExternalSession. On success, the
// caller is responsible for removing the SuspendedProcessRecord -- this
// function only performs the resume+kill, it does not touch persisted state.
func KillExternalOriginalProcess(checker AliveChecker, pid int32, createTimeMs int64, tmuxSession string) KillOutcome {
	if checker == nil {
		return KillOutcome{Status: KillOutcomeFailed, Err: fmt.Errorf("kill external process: AliveChecker is required")}
	}
	if !checker.IsAlive(pid, createTimeMs) {
		return KillOutcome{Status: KillOutcomeAlreadyGone}
	}

	// Best-effort: the process was SIGSTOP'd at commit time. It must be
	// running again before the kill signal can take effect. Ignore errors --
	// if the process already exited or was never actually stopped, the kill
	// below will simply be a no-op / fail on its own terms.
	_ = ResumeOriginalProcess(pid)

	throwaway := &Instance{
		InstanceType: InstanceTypeExternal,
		ExternalMetadata: &ExternalInstanceMetadata{
			TmuxSessionName: tmuxSession,
		},
	}
	if err := throwaway.KillExternalSession(); err != nil {
		return KillOutcome{Status: KillOutcomeFailed, Err: err}
	}

	return KillOutcome{Status: KillOutcomeKilled}
}
