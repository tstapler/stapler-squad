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

func TestWatch_should_DumpWhenTheOpOutlivesTheDeadline_And_NotWhenStopped(t *testing.T) {
	t.Parallel()
	fired := make(chan string, 2)
	dump := func(name string, _ time.Duration) { fired <- name }

	stopped := watch("stopped", time.Hour, dump)
	stopped()
	watch("hung", 5*time.Millisecond, dump)

	select {
	case name := <-fired:
		if name != "hung" {
			t.Fatalf("dump fired for %q, want hung", name)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no dump for an op outliving its deadline")
	}
}
