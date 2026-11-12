//go:build !darwin && !linux

package gitwt

import (
	"context"
	"sync"
)

// startTreeWatch is a no-op off darwin and Linux: the periodic fingerprint loop remains
// the working-tree signal.
func (d *WorktreeChangeDetector) startTreeWatch(context.Context, *sync.WaitGroup) {}
