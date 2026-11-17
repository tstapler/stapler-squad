// Package instance mimics session/instance/hot_restore.go, a file in the automated-lifecycle set.
package instance

type Instance struct{ ArchivedAt *int }

func (i *Instance) Start(first bool) error { return nil }
func (i *Instance) IsArchived() bool       { return false }

func unguardedStart(i *Instance) {
	_ = i.Start(false) // want `Start\(false\) revives a session without a preceding`
}

func guardedStart(i *Instance) {
	if i.IsArchived() {
		return
	}
	_ = i.Start(false)
}
