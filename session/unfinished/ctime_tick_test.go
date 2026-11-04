package unfinished

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ctimeTicker waits for the filesystem's coarse timestamp clock to advance.
// Linux before 6.13 stamps ctime/mtime from a jiffy-granular clock (4-10ms), so
// two mutations in the same tick get an identical ctime. Tests that inject a
// fake clock to get past the racy window skip the production guard against
// exactly that, so they must separate their mutations by a tick themselves.
// ctime cannot be set from user space, hence waiting (bounded) rather than
// stamping.
type ctimeTicker struct {
	t     testing.TB
	probe string
}

func newCtimeTicker(t testing.TB) *ctimeTicker {
	t.Helper()
	return &ctimeTicker{t: t, probe: filepath.Join(t.TempDir(), "probe")}
}

func (c *ctimeTicker) ctimeNs() int64 {
	c.t.Helper()
	if err := os.WriteFile(c.probe, nil, 0o644); err != nil {
		c.t.Fatal(err)
	}
	st, ok := statFileStamp(c.probe)
	if !ok {
		c.t.Fatal("probe file is not a regular file")
	}
	return st.CtimeNs
}

// next returns once any mutation made afterwards gets a ctime later than every
// mutation made before the call.
func (c *ctimeTicker) next() {
	c.t.Helper()
	start := c.ctimeNs()
	deadline := time.Now().Add(time.Second)
	for c.ctimeNs() <= start {
		if time.Now().After(deadline) {
			c.t.Fatal("filesystem ctime did not advance within 1s")
		}
		runtime.Gosched()
	}
}
