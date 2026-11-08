package termlat

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTest() (*Recorder, *fakeClock) {
	c := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	return New(c.now), c
}

func TestPercentile_should_UseNearestRank(t *testing.T) {
	t.Parallel()
	vals := []float64{5, 1, 3, 2, 4, 6, 7, 8, 9, 10}
	assert.Equal(t, 5.0, Percentile(vals, 50))
	assert.Equal(t, 10.0, Percentile(vals, 95))
	assert.Equal(t, 0.0, Percentile(nil, 95))
}

func TestReport_should_SplitByBackendAndListener(t *testing.T) {
	t.Parallel()
	r, _ := newTest()
	r.Record(StageEchoPaint, Labels{"tymux", "local"}, 10*time.Millisecond)
	r.Record(StageEchoPaint, Labels{"tmux", "remote"}, 90*time.Millisecond)
	r.Record(StageEchoPaint, Labels{"bogus", "weird"}, 5*time.Millisecond)

	rep := r.Report()
	require.Len(t, rep, 3)
	got := map[string]float64{}
	for _, s := range rep {
		got[s.Backend+"/"+s.Listener] = s.P95
	}
	assert.Equal(t, 10.0, got["tymux/local"])
	assert.Equal(t, 90.0, got["tmux/remote"])
	assert.Equal(t, 5.0, got["unknown/unknown"], "unknown labels fold to unknown, never a new series")
}

func TestReport_should_DropSamplesOlderThanTTL(t *testing.T) {
	t.Parallel()
	r, c := newTest()
	r.Record(StageInputTotal, Labels{"tmux", "local"}, time.Second)
	c.advance(sampleTTL + time.Second)
	r.Record(StageInputTotal, Labels{"tmux", "local"}, time.Millisecond)
	s := r.Report()[0]
	assert.Equal(t, 1, s.Count)
	assert.Equal(t, 1.0, s.P95)
	assert.EqualValues(t, 2, s.Total)
}

func TestRecordValue_should_IgnoreNegative(t *testing.T) {
	t.Parallel()
	r, _ := newTest()
	r.RecordValue(StageOutputQueue, Labels{}, -1)
	assert.Empty(t, r.Report())
}

func TestAlerting_should_FlagStageWhenP95ExceedsLimit(t *testing.T) {
	t.Parallel()
	r, c := newTest()
	l := Labels{"tymux", "local"}
	for i := 0; i < alertMinN; i++ {
		r.Record(StageEchoPaint, l, 400*time.Millisecond)
	}
	c.advance(alertCheckGap + time.Second)
	r.Record(StageEchoPaint, l, 400*time.Millisecond)
	require.Len(t, r.Alerting(), 1)

	fast, c2 := newTest()
	for i := 0; i < alertMinN+1; i++ {
		fast.Record(StageEchoPaint, l, 20*time.Millisecond)
		c2.advance(alertCheckGap)
	}
	assert.Empty(t, fast.Alerting())
}

func TestRecord_should_BeSafeConcurrently(t *testing.T) {
	t.Parallel()
	r, _ := newTest()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				r.Record(StageOutputSend, Labels{"tmux", "local"}, time.Duration(j)*time.Microsecond)
				_ = r.Report()
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 16000, r.Report()[0].Total)
}

func TestCountResync_should_BoundLabelsAndSortRows(t *testing.T) {
	t.Parallel()
	r, _ := newTest()
	r.CountResync(ResyncVisibility, "tymux")
	r.CountResync(ResyncVisibility, "tymux")
	r.CountResync(ResyncSkippedFresh, "tmux")
	r.CountResync("made-up-by-a-client", "weird")

	got := r.ResyncCounts()
	require.Len(t, got, 3)
	assert.Equal(t, ResyncCount{Reason: "skipped_fresh", Backend: "tmux", Total: 1}, got[0])
	assert.Equal(t, ResyncCount{Reason: "unknown", Backend: "unknown", Total: 1}, got[1])
	assert.Equal(t, ResyncCount{Reason: "visibility", Backend: "tymux", Total: 2}, got[2])
}
