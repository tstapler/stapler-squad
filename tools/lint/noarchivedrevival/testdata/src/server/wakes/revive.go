// Package wakes mimics server/wakes/revive.go, a file in the automated-lifecycle set.
package wakes

type target struct{}

func (t *target) Start(first bool) error { return nil }
func (t *target) RecoverFromStopped()    {}
func (t *target) IsArchived() bool       { return false }

// reviveStoppedOwner is the shape the real file must keep: its own archival
// check precedes both revival calls.
func reviveStoppedOwner(t *target) error {
	if t.IsArchived() {
		return nil
	}
	t.RecoverFromStopped()
	return t.Start(false)
}

// A guard that lives only in a caller is not enough for an exported entry.
func ReviveWithoutOwnGuard(t *target) error {
	t.RecoverFromStopped() // want `RecoverFromStopped\(\) revives`
	return t.Start(false)  // want `Start\(false\) revives`
}

func guardAfterTheCall(t *target) error {
	err := t.Start(false) // want `Start\(false\) revives`
	if t.IsArchived() {
		return nil
	}
	return err
}
