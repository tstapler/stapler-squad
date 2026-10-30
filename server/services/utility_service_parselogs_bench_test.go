package services

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// parseLogsReference is the original full-parse-every-line implementation,
// kept as the equivalence oracle and benchmark baseline for parseLogs.
func parseLogsReference(reader io.Reader, req *sessionv1.GetLogsRequest, sessionFilter string) (*parseLogsResult, error) {
	var entries []*sessionv1.LogEntry
	scanner := bufio.NewScanner(reader)
	limit := 100
	if req.Limit != nil && *req.Limit > 0 {
		limit = int(*req.Limit)
	}
	offset := 0
	if req.Offset != nil && *req.Offset > 0 {
		offset = int(*req.Offset)
	}
	filters := newLogLineFilters(req, sessionFilter)
	for scanner.Scan() {
		parsed, ok := parseLogLine(scanner.Text())
		if !ok || !filters.matches(parsed) {
			continue
		}
		entries = append(entries, &sessionv1.LogEntry{
			Timestamp: timestamppb.New(parsed.Timestamp),
			Level:     parsed.Level,
			Message:   parsed.Message,
			Source:    &parsed.Source,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	totalCount := len(entries)
	if offset >= len(entries) {
		return &parseLogsResult{Entries: []*sessionv1.LogEntry{}, TotalCount: totalCount}, nil
	}
	end := offset + limit
	if end > len(entries) {
		end = len(entries)
	}
	return &parseLogsResult{Entries: entries[offset:end], TotalCount: totalCount, HasMore: end < len(entries)}, nil
}

// buildLogFixture returns n lines mixing JSON (several attrs, 5 sessions,
// 4 levels), legacy plain-text, and garbage lines.
func buildLogFixture(n int) string {
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	base := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	var b strings.Builder
	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		switch {
		case i%50 == 7:
			fmt.Fprintf(&b, "[inst] %s:%s some legacy message %d\n", levels[i%4], ts.Local().Format("2006/01/02 15:04:05 file.go:12"), i)
		case i%97 == 3:
			b.WriteString("not a log line at all\n")
		case i%61 == 5:
			fmt.Fprintf(&b, `{"time":%q,"level":"INFO","msg":"no session","session":5,"token":"abc"}`+"\n", ts.Format(time.RFC3339Nano))
		default:
			fmt.Fprintf(&b, `{"time":%q,"level":%q,"msg":"handled request %d","session":"sess-%d","component":"server","duration_ms":%d,"path":"/api/v1/things/%d","ok":true}`+"\n",
				ts.Format(time.RFC3339Nano), levels[i%4], i, i%5, i%1000, i)
		}
	}
	return b.String()
}

func i32(v int32) *int32   { return &v }
func str(v string) *string { return &v }

func TestParseLogs_should_MatchReference_When_FiltersAndPaginationVary(t *testing.T) {
	t.Parallel()
	fixture := buildLogFixture(3000)
	start := timestamppb.New(time.Date(2026, 8, 25, 0, 5, 0, 0, time.UTC))
	end := timestamppb.New(time.Date(2026, 8, 25, 0, 40, 0, 0, time.UTC))

	cases := []struct {
		name    string
		req     *sessionv1.GetLogsRequest
		session string
	}{
		{"no filters default limit", &sessionv1.GetLogsRequest{}, ""},
		{"limit and offset", &sessionv1.GetLogsRequest{Limit: i32(25), Offset: i32(40)}, ""},
		{"offset beyond end", &sessionv1.GetLogsRequest{Limit: i32(10), Offset: i32(100000)}, ""},
		{"offset equals total", &sessionv1.GetLogsRequest{Limit: i32(10), Offset: i32(3000)}, ""},
		{"level repeated", &sessionv1.GetLogsRequest{Levels: []string{"error", "WARN"}, Limit: i32(50)}, ""},
		{"level single legacy field", &sessionv1.GetLogsRequest{Level: str("info"), Limit: i32(7), Offset: i32(3)}, ""},
		{"session", &sessionv1.GetLogsRequest{Limit: i32(1000)}, "sess-2"},
		{"session and level", &sessionv1.GetLogsRequest{Levels: []string{"ERROR"}, Limit: i32(20), Offset: i32(5)}, "sess-3"},
		{"session no match", &sessionv1.GetLogsRequest{}, "does-not-exist"},
		{"time range", &sessionv1.GetLogsRequest{StartTime: start, EndTime: end, Limit: i32(500)}, ""},
		{"search message", &sessionv1.GetLogsRequest{SearchQuery: str("HANDLED request 12"), Limit: i32(100)}, ""},
		{"search folded attr", &sessionv1.GetLogsRequest{SearchQuery: str("duration_ms=17"), Limit: i32(100), Offset: i32(2)}, ""},
		{"search plus session plus level", &sessionv1.GetLogsRequest{SearchQuery: str("/things/1"), Levels: []string{"info"}, Limit: i32(30)}, "sess-1"},
		{"search legacy", &sessionv1.GetLogsRequest{SearchQuery: str("legacy message"), Limit: i32(10)}, ""},
		{"search redacted attr", &sessionv1.GetLogsRequest{SearchQuery: str("token=<redacted>"), Limit: i32(10)}, ""},
		{"limit one", &sessionv1.GetLogsRequest{Limit: i32(1)}, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want, err := parseLogsReference(strings.NewReader(fixture), tc.req, tc.session)
			require.NoError(t, err)
			got, err := parseLogs(strings.NewReader(fixture), tc.req, tc.session)
			require.NoError(t, err)
			require.Equal(t, want.TotalCount, got.TotalCount)
			require.Equal(t, want.HasMore, got.HasMore)
			require.Len(t, got.Entries, len(want.Entries))
			for i := range want.Entries {
				require.Equal(t, want.Entries[i].Message, got.Entries[i].Message, "entry %d", i)
				require.Equal(t, want.Entries[i].Level, got.Entries[i].Level, "entry %d", i)
				require.Equal(t, want.Entries[i].GetSource(), got.Entries[i].GetSource(), "entry %d", i)
				require.True(t, want.Entries[i].Timestamp.AsTime().Equal(got.Entries[i].Timestamp.AsTime()), "entry %d", i)
			}
		})
	}
}

func BenchmarkParseLogs(b *testing.B) {
	scenarios := []struct {
		name    string
		req     *sessionv1.GetLogsRequest
		session string
	}{
		{"session", &sessionv1.GetLogsRequest{Limit: i32(100)}, "sess-2"},
		{"level", &sessionv1.GetLogsRequest{Levels: []string{"ERROR"}, Limit: i32(100)}, ""},
		{"nofilter", &sessionv1.GetLogsRequest{Limit: i32(100)}, ""},
		{"search", &sessionv1.GetLogsRequest{SearchQuery: str("things/99"), Limit: i32(100)}, ""},
	}
	impls := []struct {
		name string
		fn   func(io.Reader, *sessionv1.GetLogsRequest, string) (*parseLogsResult, error)
	}{{"old", parseLogsReference}, {"new", parseLogs}}
	for _, n := range []int{10_000, 100_000} {
		fixture := buildLogFixture(n)
		for _, sc := range scenarios {
			for _, impl := range impls {
				b.Run(fmt.Sprintf("%s/lines=%d/%s", impl.name, n, sc.name), func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						if _, err := impl.fn(strings.NewReader(fixture), sc.req, sc.session); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		}
	}
}
