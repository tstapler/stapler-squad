// Package notimesleeptest is a deliberate notimesleeptest violation; never import it.
package notimesleeptest

import (
	"testing"
	"time"
)

func TestViolation(t *testing.T) {
	time.Sleep(time.Millisecond)
}
