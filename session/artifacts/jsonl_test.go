package artifacts

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func toolUseLine(t *testing.T, ts, id, command string) []byte {
	t.Helper()
	input, _ := json.Marshal(map[string]string{"command": command})
	return mustLine(t, map[string]any{
		"type": "assistant", "timestamp": ts,
		"message": map[string]any{"role": "assistant", "content": []map[string]any{
			{"type": "tool_use", "id": id, "name": "Bash", "input": json.RawMessage(input)},
		}},
	})
}

func toolResultLine(t *testing.T, ts, id, text string) []byte {
	t.Helper()
	return mustLine(t, map[string]any{
		"type": "user", "timestamp": ts,
		"message": map[string]any{"role": "user", "content": []map[string]any{
			{"type": "tool_result", "tool_use_id": id, "content": text},
		}},
	})
}

func writeToolUseLine(t *testing.T, ts, id, toolName string, input map[string]string) []byte {
	t.Helper()
	raw, _ := json.Marshal(input)
	return mustLine(t, map[string]any{
		"type": "assistant", "timestamp": ts,
		"message": map[string]any{"role": "assistant", "content": []map[string]any{
			{"type": "tool_use", "id": "w1", "name": toolName, "input": json.RawMessage(raw)},
		}},
	})
}

func mustLine(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func scanLines(lines ...[]byte) *scanFindings {
	var f scanFindings
	for _, l := range lines {
		f.addLine(l)
	}
	f.finish()
	return &f
}

func prRecords(f *scanFindings) []PRRecord {
	var out []PRRecord
	for _, r := range f.records {
		if p, ok := r.(PRRecord); ok {
			out = append(out, p)
		}
	}
	return out
}

func TestExtractor_ShouldEmitAuthoredPRRecord_WhenToolUsePairedWithResult(t *testing.T) {
	t.Parallel()
	ts := "2026-11-04T10:15:00.000Z"
	f := scanLines(
		toolUseLine(t, "2026-11-04T10:14:00.000Z", "t1", `gh pr create --base feat-a --head feat-b --title "x"`),
		toolResultLine(t, ts, "t1", "https://github.com/acme/app/pull/12\n"),
	)
	prs := prRecords(f)
	if len(prs) != 1 {
		t.Fatalf("want one PR record, got %#v", f.records)
	}
	want := PRRecord{Host: "github.com", Owner: "acme", Repo: "app", Number: 12, Source: PRSourceCreated,
		Branch: "feat-b", Base: "feat-a", FirstSeenAt: time.Date(2026, 11, 4, 10, 15, 0, 0, time.UTC)}
	if prs[0] != want {
		t.Fatalf("got %#v\nwant %#v", prs[0], want)
	}
	if len(f.prURLs) != 1 { // v1 list stays populated
		t.Fatalf("v1 pr_urls = %v", f.prURLs)
	}
}

func TestExtractor_ShouldMarkMentioned_WhenGhPrViewOnly(t *testing.T) {
	t.Parallel()
	f := scanLines(
		toolUseLine(t, "2026-11-04T10:00:00Z", "t1", "gh pr view 99"),
		toolResultLine(t, "2026-11-04T10:00:01Z", "t1", "title: x\nurl: https://github.com/acme/app/pull/99"),
	)
	prs := prRecords(f)
	if len(prs) != 1 || prs[0].Source != PRSourceMentioned || prs[0].Number != 99 {
		t.Fatalf("got %#v", prs)
	}
}

func TestExtractor_ShouldMarkMentioned_WhenResultHasNoMatchingToolUse(t *testing.T) {
	t.Parallel()
	f := scanLines(toolResultLine(t, "2026-11-04T10:00:01Z", "orphan", "https://github.com/acme/app/pull/7"))
	if prs := prRecords(f); len(prs) != 1 || prs[0].Source != PRSourceMentioned {
		t.Fatalf("got %#v", prs)
	}
}

func TestScan_ShouldPairCommandWithResult_WhenCommandRecordCaptured(t *testing.T) {
	t.Parallel()
	f := scanLines(
		toolUseLine(t, "2026-11-04T10:00:00Z", "t1", `git commit -m "fix: x"`),
		toolResultLine(t, "2026-11-04T10:00:02Z", "t1", "[main 1a2b3c4] fix: x\n 1 file changed"),
		toolUseLine(t, "2026-11-04T10:01:00Z", "t2", `go get example.com/x@v1`), // never answered
	)
	var cmds []CommandRecord
	for _, r := range f.records {
		if c, ok := r.(CommandRecord); ok {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) != 2 {
		t.Fatalf("want 2 command records, got %#v", cmds)
	}
	if cmds[0].Type != "git_commit" || cmds[0].Result == "" || cmds[0].FirstSeenAt.IsZero() {
		t.Fatalf("paired command lost its result: %#v", cmds[0])
	}
	if cmds[1].Type != "package_install" || cmds[1].Result != "" {
		t.Fatalf("unanswered command: %#v", cmds[1])
	}
}

func TestExtractor_ShouldTruncateResult_WhenLong(t *testing.T) {
	t.Parallel()
	long := fmt.Sprintf("%01000d", 0)
	f := scanLines(
		toolUseLine(t, "2026-11-04T10:00:00Z", "t1", `git commit -m "x"`),
		toolResultLine(t, "2026-11-04T10:00:02Z", "t1", long),
	)
	for _, r := range f.records {
		if c, ok := r.(CommandRecord); ok && len(c.Result) > MaxResultBytes {
			t.Fatalf("result %d bytes", len(c.Result))
		}
	}
}
