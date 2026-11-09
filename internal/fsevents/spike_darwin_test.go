//go:build darwin

package fsevents

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// TestSpikeEventVolume measures FSEvents events/sec over a repo while a command
// runs, to fill research/spike-event-volume.md. Opt-in:
//
//	FSEVENTS_SPIKE_REPO=$PWD FSEVENTS_SPIKE_CMD='go build .' go test ./internal/fsevents -run SpikeEventVolume -v
func TestSpikeEventVolume(t *testing.T) {
	repo, cmdline := os.Getenv("FSEVENTS_SPIKE_REPO"), os.Getenv("FSEVENTS_SPIKE_CMD")
	if repo == "" || cmdline == "" {
		t.Skip("set FSEVENTS_SPIKE_REPO and FSEVENTS_SPIKE_CMD to run")
	}
	var n, rescans atomic.Int64
	s := startTestStream(t, Config{
		Paths:     []string{repo},
		OnEvents:  func(e []Event) { n.Add(int64(len(e))) },
		OnRescan:  func(Event) { rescans.Add(1) },
		SinceWhen: SinceNow,
	})

	c := safeexec.CommandContext(context.Background(), "sh", "-c", cmdline)
	c.Dir = repo
	start := time.Now()
	out, err := c.CombinedOutput()
	s.FlushSync()
	el := time.Since(start)
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	t.Logf("cmd=%q events=%d rescan_events=%d elapsed=%s rate=%.0f/s", cmdline, n.Load(), rescans.Load(), el, float64(n.Load())/el.Seconds())
}
