package artifacts

import (
	"context"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// MetricBlobBytes is the histogram of stored artifact blob sizes.
const MetricBlobBytes = "ssq_artifact_blob_bytes"

var (
	blobBytesOnce sync.Once
	blobBytes     metric.Int64Histogram
	maxBlobSeen   atomic.Int64
)

// MaxBlobBytesObserved is the largest blob size this process has stored.
func MaxBlobBytesObserved() int { return int(maxBlobSeen.Load()) }

func observeBlobBytes(n int) {
	blobBytesOnce.Do(func() {
		blobBytes, _ = telemetry.GetMeter().Int64Histogram(MetricBlobBytes, metric.WithUnit("By"))
	})
	if blobBytes != nil {
		blobBytes.Record(context.Background(), int64(n))
	}
	for {
		cur := maxBlobSeen.Load()
		if int64(n) <= cur || maxBlobSeen.CompareAndSwap(cur, int64(n)) {
			return
		}
	}
}
