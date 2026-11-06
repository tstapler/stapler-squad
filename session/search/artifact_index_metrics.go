package search

import (
	"context"
	"sync/atomic"

	"go.opentelemetry.io/otel/metric"

	"github.com/tstapler/stapler-squad/telemetry"
)

// MetricArtifactIndexDocs is the gauge of documents in the artifact index.
const MetricArtifactIndexDocs = "ssq_artifact_index_docs"

var (
	artifactIndexDocs      atomic.Int64
	artifactIndexDocsGauge = func() metric.Int64Gauge {
		g, _ := telemetry.GetMeter().Int64Gauge(MetricArtifactIndexDocs)
		return g
	}()
)

func recordArtifactIndexDocs(n int) {
	artifactIndexDocs.Store(int64(n))
	if artifactIndexDocsGauge != nil {
		artifactIndexDocsGauge.Record(context.Background(), int64(n))
	}
}

// ArtifactIndexDocsObserved is the last value reported to ssq_artifact_index_docs.
func ArtifactIndexDocsObserved() int64 { return artifactIndexDocs.Load() }
