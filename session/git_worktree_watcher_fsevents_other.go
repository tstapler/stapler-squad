//go:build !darwin

package session

import (
	"context"
	"sync"
)

// startTreeWatch is a no-op off darwin: the periodic fingerprint loop remains
// the working-tree signal (inotify on Linux has no cheap recursive equivalent).
func (d *WorktreeChangeDetector) startTreeWatch(context.Context, *sync.WaitGroup) {}
