package services

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const oddT = `"2026-01-01T00:00:00Z"`

// oddLogLines are malformed or unusual lines the old map[string]any decoder
// handled in specific ways; parseLogs must treat each identically.
var oddLogLines = map[string]string{
	"case-variant Level":             `{"time":` + oddT + `,"msg":"a","Level":"ERROR"}`,
	"case-variant Session":           `{"time":` + oddT + `,"msg":"a","Session":"S"}`,
	"case-variant duplicate session": `{"time":` + oddT + `,"msg":"a","session":"S","SESSION":"T"}`,
	"case-variant MSG":               `{"time":` + oddT + `,"MSG":"a"}`,
	"case-variant Time":              `{"Time":` + oddT + `,"msg":"a"}`,
	"case-variant Time after time":   `{"time":` + oddT + `,"msg":"a","Time":"3s"}`,
	"msg then null msg":              `{"time":` + oddT + `,"msg":"a","msg":null}`,
	"null msg then msg":              `{"time":` + oddT + `,"msg":null,"msg":"a"}`,
	"time then wrong-typed time":     `{"time":` + oddT + `,"time":5,"msg":"a"}`,
	"wrong-typed time then time":     `{"time":5,"time":` + oddT + `,"msg":"a"}`,
	"duplicate session S then T":     `{"time":` + oddT + `,"msg":"a","session":"S","session":"T"}`,
	"duplicate level":                `{"time":` + oddT + `,"msg":"a","level":"ERROR","level":"INFO"}`,
	"escaped session":                `{"time":` + oddT + `,"msg":"a","session":"SA"}`,
	"escaped key":                    `{"time":` + oddT + `,"msg":"a","session":"S"}`,
	"invalid utf8 session":           "{\"time\":" + oddT + ",\"msg\":\"a\",\"session\":\"\xff\"}",
	"missing level":                  `{"time":` + oddT + `,"msg":"a"}`,
	"lowercase level":                `{"time":` + oddT + `,"msg":"a","level":"error"}`,
	"wrong-typed level":              `{"time":` + oddT + `,"msg":"a","level":5}`,
	"null level":                     `{"time":` + oddT + `,"msg":"a","level":null}`,
	"wrong-typed session":            `{"time":` + oddT + `,"msg":"a","session":5}`,
	"null session":                   `{"time":` + oddT + `,"msg":"a","session":null}`,
	"invalid timestamp":              `{"time":"yesterday","msg":"a"}`,
	"empty time":                     `{"time":"","msg":"a"}`,
	"missing msg":                    `{"time":` + oddT + `}`,
	"non-string msg":                 `{"time":` + oddT + `,"msg":5}`,
	"overflow number":                `{"time":` + oddT + `,"msg":"a","d":1e999}`,
	"nested overflow number":         `{"time":` + oddT + `,"msg":"a","d":[1,{"x":-1e999}]}`,
	"large but valid number":         `{"time":` + oddT + `,"msg":"a","d":1e300}`,
	"nested structure":               `{"time":` + oddT + `,"msg":"a","d":{"x":[1,2,{"y":"}"}],"z":true,"n":null}}`,
	"braces in string attr":          `{"time":` + oddT + `,"msg":"a","x":"\"}{\\"}`,
	"leading spaces":                 `   {"time":` + oddT + `,"msg":"a","session":"S"}`,
	"leading tab":                    "\t{\"time\":" + oddT + ",\"msg\":\"a\",\"level\":\"ERROR\"}",
	"inner whitespace":               "{ \"time\" : " + oddT + " ,\n \"msg\" : \"a\" , \"session\" : \"S\" }",
	"trailing garbage":               `{"time":` + oddT + `,"msg":"a"} x`,
	"trailing comma":                 `{"time":` + oddT + `,"msg":"a",}`,
	"empty object":                   `{}`,
	"truncated":                      `{"time":` + oddT + `,"msg":"a`,
	"legacy-looking":                 `[inst] ERROR:2026/01/01 00:00:00 file.go:1: legacy S`,
	"search hit in attr":             `{"time":` + oddT + `,"msg":"a","needle":"found"}`,
}

func TestParseLogs_should_MatchReference_When_LinesAreOdd(t *testing.T) {
	t.Parallel()
	start := timestamppb.New(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC))
	end := timestamppb.New(time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC))
	type filter struct {
		name    string
		req     *sessionv1.GetLogsRequest
		session string
	}
	filters := []filter{
		{"none", &sessionv1.GetLogsRequest{}, ""},
		{"level ERROR", &sessionv1.GetLogsRequest{Levels: []string{"ERROR"}}, ""},
		{"level INFO", &sessionv1.GetLogsRequest{Levels: []string{"INFO"}}, ""},
		{"session S", &sessionv1.GetLogsRequest{}, "S"},
		{"session T", &sessionv1.GetLogsRequest{}, "T"},
		{"session SA", &sessionv1.GetLogsRequest{}, "SA"},
		{"session replacement char", &sessionv1.GetLogsRequest{}, "�"},
		{"time range excluding", &sessionv1.GetLogsRequest{StartTime: start, EndTime: end}, ""},
		{"search", &sessionv1.GetLogsRequest{SearchQuery: str("found")}, ""},
	}
	for name, line := range oddLogLines {
		for _, f := range filters {
			name, line, f := name, line, f
			t.Run(name+"/"+f.name, func(t *testing.T) {
				t.Parallel()
				assertParseLogsMatchesReference(t, line+"\n", f.req, f.session)
			})
		}
	}
}

func TestParseLogs_should_MatchReference_When_OddLinesAreMixedWithOutOfOrderTimestamps(t *testing.T) {
	t.Parallel()
	lines := []string{
		`{"time":"2026-03-01T00:00:00Z","msg":"late","level":"ERROR"}`,
		`{"time":"2026-01-01T00:00:00Z","msg":"early","level":"ERROR"}`,
		`{"time":"2026-02-01T00:00:00Z","msg":"mid","Level":"ERROR"}`,
		oddLogLines["overflow number"],
		oddLogLines["case-variant duplicate session"],
		oddLogLines["leading spaces"],
	}
	start := timestamppb.New(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC))
	for _, req := range []*sessionv1.GetLogsRequest{
		{},
		{Levels: []string{"ERROR"}, Limit: i32(2)},
		{StartTime: start, Limit: i32(2), Offset: i32(1)},
	} {
		assertParseLogsMatchesReference(t, strings.Join(lines, "\n")+"\n", req, "")
	}
}

// A line longer than bufio.Scanner's 64KB token limit aborts both
// implementations with an error.
func TestParseLogs_should_MatchReference_When_LineExceedsScannerLimit(t *testing.T) {
	t.Parallel()
	long := `{"time":` + oddT + `,"msg":"` + strings.Repeat("x", 70000) + `"}`
	content := `{"time":` + oddT + `,"msg":"ok"}` + "\n" + long + "\n"
	_, wantErr := parseLogsReference(strings.NewReader(content), &sessionv1.GetLogsRequest{}, "")
	_, gotErr := parseLogs(strings.NewReader(content), &sessionv1.GetLogsRequest{}, "")
	require.Error(t, wantErr)
	require.Error(t, gotErr)
}

func assertParseLogsMatchesReference(t *testing.T, content string, req *sessionv1.GetLogsRequest, session string) {
	t.Helper()
	want, wantErr := parseLogsReference(strings.NewReader(content), req, session)
	got, gotErr := parseLogs(strings.NewReader(content), req, session)
	require.Equal(t, wantErr != nil, gotErr != nil)
	if wantErr != nil {
		return
	}
	require.Equal(t, want.TotalCount, got.TotalCount, "TotalCount")
	require.Equal(t, want.HasMore, got.HasMore, "HasMore")
	require.Len(t, got.Entries, len(want.Entries))
	for i := range want.Entries {
		require.Equal(t, want.Entries[i].Message, got.Entries[i].Message)
		require.Equal(t, want.Entries[i].Level, got.Entries[i].Level)
		require.Equal(t, want.Entries[i].GetSource(), got.Entries[i].GetSource())
		require.True(t, want.Entries[i].Timestamp.AsTime().Equal(got.Entries[i].Timestamp.AsTime()))
	}
}

// FuzzParseLogsMatchesReference feeds arbitrary single lines to both
// implementations. Run with: go test ./server/services -run xxx -fuzz FuzzParseLogsMatchesReference
func FuzzParseLogsMatchesReference(f *testing.F) {
	for _, line := range oddLogLines {
		f.Add(line)
	}
	f.Fuzz(func(t *testing.T, line string) {
		if strings.ContainsAny(line, "\r\n") {
			t.Skip("multi-line input is a different scenario")
		}
		for _, session := range []string{"", "S"} {
			assertParseLogsMatchesReference(t, line+"\n",
				&sessionv1.GetLogsRequest{Levels: []string{"ERROR", "INFO"}}, session)
		}
	})
}
