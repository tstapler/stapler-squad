package wakes

import (
	"fmt"

	"github.com/gofrs/flock"
)

// LeaderGuard keeps two processes from running the dispatcher against one
// database during a restart overlap (docs/explanation/service-restart-orphan-process.md).
type LeaderGuard interface {
	// TryAcquire takes leadership without blocking. ok=false means another
	// holder has it; release gives it up.
	TryAcquire() (release func(), ok bool, err error)
}

// NoLeader is the guard for single-process use and tests: always leader.
type NoLeader struct{}

func (NoLeader) TryAcquire() (func(), bool, error) { return func() {}, true, nil }

// FileLeader is a LeaderGuard backed by an exclusive flock on Path. The OS
// drops the lock when the holder exits, even uncleanly, so a lost process
// cannot hold leadership forever.
type FileLeader struct{ Path string }

// TryAcquire locks Path without blocking.
func (l FileLeader) TryAcquire() (func(), bool, error) {
	lock := flock.New(l.Path)
	ok, err := lock.TryLock()
	if err != nil {
		return nil, false, fmt.Errorf("lock %s: %w", l.Path, err)
	}
	if !ok {
		return nil, false, nil
	}
	return func() { _ = lock.Unlock() }, true, nil
}
