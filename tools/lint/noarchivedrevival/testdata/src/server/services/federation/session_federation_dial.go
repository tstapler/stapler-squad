// Package federation mimics server/services/federation/session_federation_dial.go,
// which is in lifecycleFiles: federation is read-only and must never revive a
// session, so any Start(false) there needs an archival guard.
package federation

type inst struct{}

func (i *inst) Start(first bool) error { return nil }
func (i *inst) IsArchived() bool       { return false }

func unguardedRevive(i *inst) error {
	return i.Start(false) // want `Start\(false\) revives`
}

func guardedRevive(i *inst) error {
	if i.IsArchived() {
		return nil
	}
	return i.Start(false)
}
