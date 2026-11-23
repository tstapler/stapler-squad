package slowops

import (
	"testing"
	"time"
)

func TestWatch_should_ReturnAStopFunc(t *testing.T) {
	t.Parallel()
	Watch("op", time.Hour)()
}

func TestLogGoroutineDump_should_NotPanic(t *testing.T) {
	t.Parallel()
	logGoroutineDump("op", time.Minute)
}
