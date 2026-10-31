//go:build !unix

package session

import (
	"os"
	"time"
)

// pollPTYBeforeRead is false: without poll(2) the loop keeps using SetReadDeadline.
const pollPTYBeforeRead = false

// waitReadable is a stub on platforms without poll(2): it reports "ready" so
// the caller falls back to a plain blocking Read, the previous behaviour.
func waitReadable(*os.File, time.Duration) (bool, error) { return true, nil }
