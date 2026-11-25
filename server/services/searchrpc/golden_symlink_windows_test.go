//go:build windows

package searchrpc

import (
	"testing"
	"time"
)

func setLinkMtime(t *testing.T, _ string, _ time.Time) { t.Helper() }
