package config

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

const (
	// InstanceLockFileName is the exclusive, process-lifetime lock file used
	// to prevent two stapler-squad server processes from running against the
	// same config/DB directory at once.
	InstanceLockFileName = "instance.lock"
	// DefaultInstanceLockTimeout bounds how long a starting process waits for
	// a prior process to release the instance lock before giving up. Matches
	// scripts/install-service.sh's wait_for_port_release budget so a normal
	// service restart doesn't spuriously fail here.
	DefaultInstanceLockTimeout = 10 * time.Second
)

// AcquireInstanceLock takes an exclusive lock on instance.lock in configDir,
// retrying until timeout elapses. Unlike the PID/port-based liveness checks
// documented in docs/explanation/service-restart-orphan-process.md, the OS
// releases a flock the moment the holding process's file descriptors close —
// including on an unclean exit or reparenting to PID 1 — so a prior process
// that launchd/systemd has lost track of still can't hold this lock forever.
//
// Returns the acquired *flock.Flock; the caller must keep it alive for the
// life of the process and Unlock() it on shutdown.
func AcquireInstanceLock(configDir string, timeout time.Duration) (*flock.Flock, error) {
	lockPath := filepath.Join(configDir, InstanceLockFileName)
	fileLock := flock.New(lockPath)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	locked, err := fileLock.TryLockContext(ctx, 200*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire instance lock at %s: %w", lockPath, err)
	}
	if !locked {
		return nil, fmt.Errorf("another stapler-squad process already holds the instance lock at %s (timed out after %s) — check for an orphaned process (see docs/explanation/service-restart-orphan-process.md)", lockPath, timeout)
	}
	return fileLock, nil
}
