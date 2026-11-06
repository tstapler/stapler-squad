package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/tokens"
)

const (
	benchSessions       = 3000
	benchTurnsPerResult = 40
)

func syntheticInsightsFixture(n int) *InsightsService {
	now := time.Now().UTC()
	models := []string{"claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"}
	results := make([]*tokens.ParseResult, 0, n)
	records := make([]tokens.SessionRecord, 0, n/2)
	for i := 0; i < n; i++ {
		r := newResult(fmt.Sprintf("uuid-%d", i), models[i%len(models)], fmt.Sprintf("/proj/%d", i%50), 1000, 500, 200, now.Add(-time.Duration(i)*time.Hour))
		for j := 0; j < benchTurnsPerResult; j++ {
			r.TurnTimeline = append(r.TurnTimeline, tokens.TurnStats{
				Model: r.PrimaryModel, Input: 100, Output: 50, CacheRead: 20, Timestamp: r.FileModTime.Add(time.Duration(j) * time.Minute),
			})
		}
		results = append(results, r)
		if i%2 == 0 {
			records = append(records, tokens.SessionRecord{SessionID: fmt.Sprintf("sess-%d", i)})
		}
	}
	svc := newInsightsFixture(results, records)
	return svc
}

// BenchmarkComputeInsightsSummary measures the uncached aggregate over a few
// thousand synthetic sessions (the cost the summaryCache amortizes).
func BenchmarkComputeInsightsSummary(b *testing.B) {
	svc := syntheticInsightsFixture(benchSessions)
	req := &sessionv1.GetInsightsSummaryRequest{IncludeOrphans: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.computeInsightsSummary(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}
