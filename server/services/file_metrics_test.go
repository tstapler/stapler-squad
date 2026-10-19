package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestFileMetrics_ShouldIncrementCountersAndObserveHistograms_When_RpcsComplete(t *testing.T) {
	cases := []struct {
		counter, histogram, outcome string
		record                      func()
	}{
		{"files.search.total", "files.search.duration_ms", string(SearchOutcomeOK), func() {
			recordSearchMetrics(context.Background(), SearchOutcomeOK, 12*time.Millisecond, 3*time.Millisecond)
		}},
		{"files.write.total", "files.write.duration_ms", string(WriteOutcomeOK), func() {
			recordWriteMetrics(context.Background(), WriteOutcomeOK, 7*time.Millisecond)
		}},
		{"files.range.total", "files.range.duration_ms", "index_pending", func() {
			recordRangeMetrics(context.Background(), "index_pending", 5*time.Millisecond)
		}},
		{"files.stat.total", "files.stat.duration_ms", "denied", func() {
			recordStatMetrics(context.Background(), connect.NewError(connect.CodePermissionDenied, nil), time.Millisecond)
		}},
	}
	for _, c := range cases {
		t.Run(c.counter, func(t *testing.T) {
			countBefore := sumForOutcome(t, collectMetric(t, c.counter), c.outcome)
			histBefore := histogramCountForOutcome(t, collectMetric(t, c.histogram), c.outcome)

			c.record()

			assert.Equal(t, countBefore+1, sumForOutcome(t, collectMetric(t, c.counter), c.outcome))
			assert.Equal(t, histBefore+1, histogramCountForOutcome(t, collectMetric(t, c.histogram), c.outcome))
		})
	}
}

func TestStatFile_ShouldRecordOkAndInvalidOutcomes_When_FileExistsOrIsMissing(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644))
	svc := realFileService(dir)
	stat := func(path string) error {
		_, err := svc.StatFile(context.Background(), connect.NewRequest(&sessionv1.StatFileRequest{SessionId: "test-session", Path: path}))
		return err
	}
	okBefore := sumForOutcome(t, collectMetric(t, "files.stat.total"), "ok")
	invalidBefore := sumForOutcome(t, collectMetric(t, "files.stat.total"), "invalid")

	require.NoError(t, stat("a.txt"))
	require.Error(t, stat("missing.txt"))

	assert.Equal(t, okBefore+1, sumForOutcome(t, collectMetric(t, "files.stat.total"), "ok"))
	assert.Equal(t, invalidBefore+1, sumForOutcome(t, collectMetric(t, "files.stat.total"), "invalid"))
}
