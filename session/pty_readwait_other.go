//go:build !unix

package session

import (
	"os"
	"time"
)

// readDeadlineTrusted: without poll(2) the read deadline is the only bound available.
const readDeadlineTrusted = true

// waitReadable is a stub on platforms without poll(2): it reports "ready" so
// the caller falls back to a plain blocking Read, the previous behaviour.
func waitReadable(*os.File, time.Duration) (bool, error) { return true, nil }
