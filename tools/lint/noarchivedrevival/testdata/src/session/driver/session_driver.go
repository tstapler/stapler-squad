// Package driver mimics session/driver/session_driver.go, which moved out of
// session/ and must stay in the automated-lifecycle set.
package driver

type Instance struct{}

func (i *Instance) IsArchived() bool { return false }

func restartForRetry(i *Instance) error { return nil }

func unguardedRetry(i *Instance) {
	_ = restartForRetry(i) // want `restartForRetry\(\) revives`
}

func guardedRetry(i *Instance) {
	if i.IsArchived() {
		return
	}
	_ = restartForRetry(i)
}
