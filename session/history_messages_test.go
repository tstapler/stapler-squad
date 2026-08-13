package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- helpers -----------------------------------------------------------------

// buildJSONLFile writes a set of conversationMessage entries as a JSONL file
// and returns the path.  Each msg is JSON-encoded on its own line.
func buildJSONLFile(t *testing.T, msgs []conversationMessage) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "conv-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close() //nolint:errcheck

	for _, m := range msgs {
		line, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return f.Name()
}

// makeMsg builds a conversationMessage with the given type and text content.
func makeMsg(typ, text string) conversationMessage {
	ts := time.Now().UTC().Format(time.RFC3339)
	role := "user"
	if typ == "assistant" {
		role = "assistant"
	}
	return conversationMessage{
		Type:      typ,
		SessionID: "test-session",
		Timestamp: ts,
		Message: struct {
			Role    string      `json:"role"`
			Model   string      `json:"model,omitempty"`
			Content interface{} `json:"content"`
		}{
			Role:    role,
			Content: text,
		},
	}
}

// makeToolUseMsg creates a non-user/non-assistant entry (tool_result, etc.)
func makeToolUseMsg() conversationMessage {
	return conversationMessage{
		Type:      "tool_result",
		SessionID: "test-session",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// --- extractMsgContent -------------------------------------------------------

func TestExtractMsgContent_StringContent(t *testing.T) {
	raw := makeMsg("user", "hello world")
	msg, ok := extractMsgContent(raw)
	if !ok {
		t.Fatal("expected ok=true for user message")
	}
	if msg.Content != "hello world" {
		t.Errorf("content = %q; want %q", msg.Content, "hello world")
	}
	if msg.Role != "user" {
		t.Errorf("role = %q; want user", msg.Role)
	}
}

func TestExtractMsgContent_ArrayContent(t *testing.T) {
	raw := conversationMessage{
		Type:      "assistant",
		SessionID: "s",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Message: struct {
			Role    string      `json:"role"`
			Model   string      `json:"model,omitempty"`
			Content interface{} `json:"content"`
		}{
			Role: "assistant",
			Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "first"},
				map[string]interface{}{"type": "tool_use", "id": "ignored"},
				map[string]interface{}{"type": "text", "text": "second"},
			},
		},
	}
	msg, ok := extractMsgContent(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(msg.Content, "first") || !strings.Contains(msg.Content, "second") {
		t.Errorf("content = %q; want both 'first' and 'second'", msg.Content)
	}
}

func TestExtractMsgContent_NonMessageType(t *testing.T) {
	raw := makeToolUseMsg()
	_, ok := extractMsgContent(raw)
	if ok {
		t.Fatal("expected ok=false for tool_result entry")
	}
}

// --- readAllMessagesFromFile -------------------------------------------------

func TestReadAllMessagesFromFile_Empty(t *testing.T) {
	path := buildJSONLFile(t, nil)
	msgs, err := readAllMessagesFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages; want 0", len(msgs))
	}
}

func TestReadAllMessagesFromFile_OnlyToolEntries(t *testing.T) {
	path := buildJSONLFile(t, []conversationMessage{
		makeToolUseMsg(),
		makeToolUseMsg(),
	})
	msgs, err := readAllMessagesFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages; want 0", len(msgs))
	}
}

func TestReadAllMessagesFromFile_MixedEntries(t *testing.T) {
	path := buildJSONLFile(t, []conversationMessage{
		makeMsg("user", "msg1"),
		makeToolUseMsg(),
		makeMsg("assistant", "reply1"),
		makeToolUseMsg(),
		makeMsg("user", "msg2"),
	})
	msgs, err := readAllMessagesFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("got %d messages; want 3", len(msgs))
	}
	wantContents := []string{"msg1", "reply1", "msg2"}
	for i, want := range wantContents {
		if msgs[i].Content != want {
			t.Errorf("msgs[%d].Content = %q; want %q", i, msgs[i].Content, want)
		}
	}
}

func TestReadAllMessagesFromFile_MalformedLines(t *testing.T) {
	// Write valid + invalid lines directly
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.jsonl")

	validLine, _ := json.Marshal(makeMsg("user", "valid"))
	content := fmt.Sprintf("%s\nnot json at all\n%s\n", validLine, validLine)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	msgs, err := readAllMessagesFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("got %d messages; want 2 (malformed lines should be skipped)", len(msgs))
	}
}

func TestReadAllMessagesFromFile_NonExistent(t *testing.T) {
	_, err := readAllMessagesFromFile("/nonexistent/path/conv.jsonl")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// --- readLastNMessagesFromFile -----------------------------------------------

func TestReadLastNMessages_LimitZeroReturnsAll(t *testing.T) {
	path := buildJSONLFile(t, []conversationMessage{
		makeMsg("user", "a"),
		makeMsg("assistant", "b"),
		makeMsg("user", "c"),
	})
	msgs, err := readLastNMessagesFromFile(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("got %d; want 3", len(msgs))
	}
}

func TestReadLastNMessages_LimitExceedsCount(t *testing.T) {
	path := buildJSONLFile(t, []conversationMessage{
		makeMsg("user", "only one"),
	})
	msgs, err := readLastNMessagesFromFile(path, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("got %d; want 1", len(msgs))
	}
	if msgs[0].Content != "only one" {
		t.Errorf("content = %q; want %q", msgs[0].Content, "only one")
	}
}

func TestReadLastNMessages_LimitAtExactCount(t *testing.T) {
	msgs := make([]conversationMessage, 5)
	for i := range msgs {
		msgs[i] = makeMsg("user", fmt.Sprintf("msg%d", i))
	}
	path := buildJSONLFile(t, msgs)

	got, err := readLastNMessagesFromFile(path, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("got %d; want 5", len(got))
	}
}

func TestReadLastNMessages_LimitReturnsLastN(t *testing.T) {
	raw := make([]conversationMessage, 20)
	for i := range raw {
		raw[i] = makeMsg("user", fmt.Sprintf("msg%d", i))
	}
	path := buildJSONLFile(t, raw)

	got, err := readLastNMessagesFromFile(path, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d; want 5", len(got))
	}
	// Should be the LAST 5 messages in chronological order
	for i, msg := range got {
		want := fmt.Sprintf("msg%d", 15+i)
		if msg.Content != want {
			t.Errorf("got[%d].Content = %q; want %q", i, msg.Content, want)
		}
	}
}

func TestReadLastNMessages_ChronologicalOrder(t *testing.T) {
	// Build 100 messages; request last 10.  Verify order is oldest-first.
	raw := make([]conversationMessage, 100)
	for i := range raw {
		raw[i] = makeMsg("user", fmt.Sprintf("item%03d", i))
	}
	path := buildJSONLFile(t, raw)

	got, err := readLastNMessagesFromFile(path, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d; want 10", len(got))
	}
	for i := range got {
		want := fmt.Sprintf("item%03d", 90+i)
		if got[i].Content != want {
			t.Errorf("got[%d].Content = %q; want %q", i, got[i].Content, want)
		}
	}
}

func TestReadLastNMessages_EmptyFile(t *testing.T) {
	path := buildJSONLFile(t, nil)
	msgs, err := readLastNMessagesFromFile(path, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d; want 0", len(msgs))
	}
}

func TestReadLastNMessages_SkipsToolEntries(t *testing.T) {
	// Interleave tool entries; the tail read must skip them and return user/assistant only.
	raw := []conversationMessage{
		makeMsg("user", "first"),
		makeToolUseMsg(),
		makeToolUseMsg(),
		makeMsg("assistant", "second"),
		makeToolUseMsg(),
		makeMsg("user", "third"),
	}
	path := buildJSONLFile(t, raw)

	got, err := readLastNMessagesFromFile(path, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d; want 2", len(got))
	}
	if got[0].Content != "second" || got[1].Content != "third" {
		t.Errorf("wrong messages: %v", got)
	}
}

func TestReadLastNMessages_LargeFile_SpansMultipleChunks(t *testing.T) {
	// Create enough messages that the file exceeds one 64 KiB chunk.
	// Each message content is ~200 bytes; 400 messages ≈ 80 KB → two chunks.
	const total = 400
	raw := make([]conversationMessage, total)
	for i := range raw {
		raw[i] = makeMsg("user", fmt.Sprintf("%04d: %s", i, strings.Repeat("x", 200)))
	}
	path := buildJSONLFile(t, raw)

	got, err := readLastNMessagesFromFile(path, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d; want 10", len(got))
	}
	for i, msg := range got {
		wantPrefix := fmt.Sprintf("%04d:", total-10+i)
		if !strings.HasPrefix(msg.Content, wantPrefix) {
			t.Errorf("got[%d] = %q; want prefix %q", i, msg.Content[:10], wantPrefix)
		}
	}
}

// ResultsMatchForwardAndReverse verifies that readLastNMessagesFromFile returns
// the same last-N entries as reading all messages and slicing the tail.
func TestReadLastNMessages_MatchesForwardRead(t *testing.T) {
	const total = 50
	raw := make([]conversationMessage, total)
	for i := range raw {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		raw[i] = makeMsg(role, fmt.Sprintf("content_%03d", i))
	}
	path := buildJSONLFile(t, raw)

	allMsgs, err := readAllMessagesFromFile(path)
	if err != nil {
		t.Fatalf("readAllMessagesFromFile: %v", err)
	}

	for _, n := range []int{1, 5, 10, total, total + 10} {
		got, err := readLastNMessagesFromFile(path, n)
		if err != nil {
			t.Fatalf("readLastNMessagesFromFile(%d): %v", n, err)
		}
		want := allMsgs
		if n < len(allMsgs) {
			want = allMsgs[len(allMsgs)-n:]
		}
		if len(got) != len(want) {
			t.Errorf("n=%d: got %d messages, want %d", n, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i].Content != want[i].Content {
				t.Errorf("n=%d, i=%d: got %q; want %q", n, i, got[i].Content, want[i].Content)
			}
		}
	}
}

// --- GetMessagesFromConversationFile (via real session history) ---------------

func TestGetMessagesFromConversationFile_NotFound(t *testing.T) {
	// Use a directory that doesn't exist as the projects dir.
	// We can't easily inject the path, so test via findConversationFilePath directly.
	_, err := findConversationFilePath("nonexistent-session-id-xyz")
	if err == nil {
		t.Fatal("expected error for non-existent session")
	}
}
