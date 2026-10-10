// Package session mimics session/health.go, a file in the automated-lifecycle set.
package session

type Status int

const Active Status = 1

type Instance struct{ ArchivedAt *int }

func (i *Instance) Start(first bool) error { return nil }
func (i *Instance) RecoverFromStopped()    {}
func (i *Instance) IsArchived() bool       { return false }

func restartForRetry(i *Instance) error { return nil }
func transitionToLocked(s *Instance, ctx int, to Status) error {
	return nil
}

func unguardedStart(i *Instance) {
	_ = i.Start(false) // want `Start\(false\) revives a session without a preceding`
}

func unguardedRecover(i *Instance) {
	i.RecoverFromStopped() // want `RecoverFromStopped\(\) revives`
}

func unguardedRetry(i *Instance) {
	_ = restartForRetry(i) // want `restartForRetry\(\) revives`
}

func unguardedTransition(i *Instance) {
	_ = transitionToLocked(i, 0, Active) // want `transitionToLocked\(\.\.\., Active\) revives`
}

func guardedByMethod(i *Instance) {
	if i.IsArchived() {
		return
	}
	_ = i.Start(false)
}

func guardedByField(i *Instance) {
	if i.ArchivedAt != nil {
		return
	}
	i.RecoverFromStopped()
}

func guardedByHoistedVar(list []*Instance) {
	for _, i := range list {
		archived := i.IsArchived()
		if archived {
			continue
		}
		_ = transitionToLocked(i, 0, Active)
	}
}

func guardAfterCallDoesNotCount(i *Instance) {
	_ = i.Start(false) // want `Start\(false\) revives`
	_ = i.IsArchived()
}

func firstTimeStartIsNotRevival(i *Instance) {
	_ = i.Start(true)
}

func suppressed(i *Instance) {
	_ = i.Start(false) //nolint:noarchivedrevival explicit user resume
}

func suppressedWithoutReason(i *Instance) {
	//nolint:noarchivedrevival
	_ = i.Start(false) // want `requires a one-line justification`
}

func guardInUnrelatedBlock(i *Instance, ok bool) {
	if ok {
		if i.IsArchived() {
			return
		}
	}
	_ = i.Start(false) // want `Start\(false\) revives`
}

func guardWrapsCall(i *Instance) {
	if !i.IsArchived() {
		_ = i.Start(false)
	}
}

func coveredHelper(i *Instance) {
	_ = i.Start(false)
}

func coveredCaller(i *Instance) {
	if i.IsArchived() {
		return
	}
	coveredHelper(i)
}

func uncoveredHelper(i *Instance) {
	_ = i.Start(false) // want `Start\(false\) revives`
}

func uncoveredCallerA(i *Instance) {
	if i.IsArchived() {
		return
	}
	uncoveredHelper(i)
}

func uncoveredCallerB(i *Instance) {
	uncoveredHelper(i)
}

func skipReason(i *Instance) (string, bool) {
	if i.ArchivedAt != nil {
		return "archived", true
	}
	return "", false
}

func notAGuardHelper(i *Instance) (string, bool) { return "", false }

func guardedByHelper(i *Instance) {
	if _, skip := skipReason(i); skip {
		return
	}
	_ = i.Start(false)
}

func nonCheckingHelperIsNotAGuard(i *Instance) {
	if _, skip := notAGuardHelper(i); skip {
		return
	}
	_ = i.Start(false) // want `Start\(false\) revives`
}

func fieldCopyIsNotAGuard(i *Instance) {
	copied := Instance{ArchivedAt: i.ArchivedAt}
	_ = copied
	_ = i.Start(false) // want `Start\(false\) revives`
}
