package log

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// serviceLogMaxBytes mirrors scripts/install-service.sh's LOG_ROTATE_MAX_BYTES.
const serviceLogMaxBytes = 20 * 1024 * 1024

// MonitorServiceLogSize periodically copytruncates the launchd/systemd-captured
// raw stdout/stderr log (~/.stapler-squad/logs/service.log) once it exceeds
// serviceLogMaxBytes. install-service.sh's rotate_log_if_large only runs at
// install/restart time -- launchd/systemd just append() to StandardOutPath
// forever otherwise -- so a crash-restart loop between installs can grow it
// unboundedly (service.log.old reached 420MB this way before the
// control-mode nil-channel panic behind that loop was fixed in 2b93ea421).
// This keeps it bounded even when the process runs for a long time between
// installs.
//
// Truncating in place (not renaming) is required: this process's own
// stdout/stderr fd stays open across the check, so only a copytruncate --
// content copied to path+".old", then the original truncated to 0 -- is
// visible to that still-open O_APPEND fd. A rename would leave the fd
// writing into the renamed file forever, since renaming a path doesn't
// affect fds already open on the underlying inode.
func MonitorServiceLogSize(ctx context.Context, interval time.Duration) {
	path, err := serviceLogPath()
	if err != nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rotateServiceLogIfLarge(path)
		}
	}
}

// serviceLogPath returns the fixed path scripts/install-service.sh's
// generated launchd plist/systemd unit always redirects stdout/stderr to.
// Deliberately not workspace/test-mode aware (unlike config.GetConfigDir) --
// the service's StandardOutPath/StandardOutput is set once at install time
// and never varies with in-process workspace state.
func serviceLogPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".stapler-squad", "logs", "service.log"), nil
}

func rotateServiceLogIfLarge(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= serviceLogMaxBytes {
		return
	}
	// #nosec G304 -- path is the fixed, non-user-controlled service log path from serviceLogPath().
	data, err := os.ReadFile(path)
	if err != nil {
		Error("service log rotation: failed to read log for copytruncate", "err", err)
		return
	}
	// Keep one extra generation (.old -> .old.1) before overwriting .old, so a
	// crash loop that straddles two rotation windows doesn't erase the
	// evidence from the first one. Best-effort: a missing/failed-to-rename
	// .old is fine, just means there's nothing to preserve yet.
	if err := os.Rename(path+".old", path+".old.1"); err != nil && !os.IsNotExist(err) {
		Warn("service log rotation: failed to age out previous .old generation", "err", err)
	}
	if err := os.WriteFile(path+".old", data, 0600); err != nil {
		Error("service log rotation: failed to write rotated log", "err", err)
		return
	}
	if err := os.Truncate(path, 0); err != nil {
		Error("service log rotation: failed to truncate log", "err", err)
	}
}
