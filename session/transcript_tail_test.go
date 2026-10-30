package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func assistantLine(uuid, text string) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":%q,"message":{"content":[{"type":"text","text":%q}]}}`, uuid, text) + "\n"
}

func writeTranscript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadLastAssistantMessage_should_ReturnLastAssistantText_When_MixedLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := assistantLine("u1", "first") + `{"type":"user","uuid":"u2","message":{"content":"hi"}}` + "\n" +
		`{"type":"assistant","uuid":"u3","message":{"content":[{"type":"tool_use","name":"Bash"}]}}` + "\n" +
		assistantLine("u4", "Should I use Postgres or SQLite?") + "not json\n"
	p := writeTranscript(t, dir, "a.jsonl", body)
	msg, found, err := ReadLastAssistantMessage(dir, p)
	if err != nil || !found || msg.UUID != "u4" || msg.Text != "Should I use Postgres or SQLite?" {
		t.Fatalf("got %+v found=%v err=%v", msg, found, err)
	}
}

func TestReadLastAssistantMessage_should_ReadOnlyTail_When_TranscriptIsHuge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	filler := assistantLine("old", strings.Repeat("x", 1000))
	var b strings.Builder
	for b.Len() < 10<<20 {
		b.WriteString(filler)
	}
	b.WriteString(assistantLine("last", "Done?"))
	p := writeTranscript(t, dir, "big.jsonl", b.String())
	start := time.Now()
	msg, found, err := ReadLastAssistantMessage(dir, p)
	if err != nil || !found || msg.UUID != "last" {
		t.Fatalf("got %+v found=%v err=%v", msg, found, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("tail read too slow: %v", time.Since(start))
	}
}

func TestReadLastAssistantMessage_should_DropPartialFirstLine_When_Truncated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Only the cut first line would parse as an assistant record if kept.
	tailOnly := assistantLine("keep", "tail text")
	pad := strings.Repeat("y", MaxTranscriptTailBytes-len(tailOnly)-10)
	body := assistantLine("cut", "head"+pad) + tailOnly
	p := writeTranscript(t, dir, "c.jsonl", body)
	msg, found, err := ReadLastAssistantMessage(dir, p)
	if err != nil || !found || msg.UUID != "keep" {
		t.Fatalf("got %+v found=%v err=%v", msg, found, err)
	}
}

func TestReadLastAssistantMessage_should_RefuseEscapes_When_PathOutsideProjectsDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "projects")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{dir, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	secret := writeTranscript(t, outside, "s.jsonl", assistantLine("u", "secret"))
	if err := os.Symlink(secret, filepath.Join(dir, "link.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linkdir")); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"absolute":       "/etc/passwd",
		"dotdot":         filepath.Join(dir, "..", "..", "etc", "passwd"),
		"dotdot-sibling": filepath.Join(dir, "..", "outside", "s.jsonl"),
		"file symlink":   filepath.Join(dir, "link.jsonl"),
		"dir symlink":    filepath.Join(dir, "linkdir", "s.jsonl"),
		"empty":          "",
		"dir itself":     dir,
	}
	for name, p := range cases {
		_, found, err := ReadLastAssistantMessage(dir, p)
		if found || err == nil {
			t.Errorf("%s: want refusal, got found=%v err=%v", name, found, err)
		}
		if name != "empty" && name != "absolute" && name != "dotdot" && !errors.Is(err, ErrTranscriptPathRejected) {
			t.Errorf("%s: want ErrTranscriptPathRejected, got %v", name, err)
		}
	}
}
