package services

import (
	"testing"
)

// --- cursor encoding / decoding ---------------------------------------------

func TestEncodeDecodeHistoryCursor_RoundTrip(t *testing.T) {
	c := historyCursor{UpdatedAtNs: 1_700_000_000_000_000_000, ID: "abc-123"}
	token := encodeHistoryCursor(c)
	if token == "" {
		t.Fatal("encodeHistoryCursor returned empty string")
	}

	got, ok := decodeHistoryCursor(token)
	if !ok {
		t.Fatalf("decodeHistoryCursor(%q) returned ok=false", token)
	}
	if got.UpdatedAtNs != c.UpdatedAtNs {
		t.Errorf("UpdatedAtNs: got %d; want %d", got.UpdatedAtNs, c.UpdatedAtNs)
	}
	if got.ID != c.ID {
		t.Errorf("ID: got %q; want %q", got.ID, c.ID)
	}
}

func TestDecodeHistoryCursor_EmptyToken(t *testing.T) {
	_, ok := decodeHistoryCursor("")
	if ok {
		t.Fatal("expected ok=false for empty token")
	}
}

func TestDecodeHistoryCursor_InvalidBase64(t *testing.T) {
	_, ok := decodeHistoryCursor("!!!not-base64!!!")
	if ok {
		t.Fatal("expected ok=false for invalid base64")
	}
}

func TestDecodeHistoryCursor_InvalidJSON(t *testing.T) {
	// base64url("not json") without padding
	token := "bm90IGpzb24"
	_, ok := decodeHistoryCursor(token)
	if ok {
		t.Fatal("expected ok=false for non-JSON payload")
	}
}

func TestEncodeHistoryCursor_IsURLSafe(t *testing.T) {
	c := historyCursor{UpdatedAtNs: int64(9_223_372_036_854_775_807), ID: "some-id-with-special/chars+=="}
	token := encodeHistoryCursor(c)
	for _, ch := range token {
		if ch == '+' || ch == '/' || ch == '=' {
			t.Errorf("token %q contains URL-unsafe char %q", token, ch)
		}
	}
}

// --- ListClaudeHistory pagination (unit tests via helper) -------------------
// These tests exercise the pagination slice logic by simulating what
// ListClaudeHistory does internally, without spinning up a real server.

type paginationResult struct {
	entries       []string // IDs in order
	nextPageToken string
}

// simulatePage applies the same pagination logic as ListClaudeHistory to a
// slice of IDs (used as both ID and sort key for simplicity).
func simulatePage(ids []string, pageSize int, pageToken string) paginationResult {
	type entry struct {
		id          string
		updatedAtNs int64
	}

	// Build fake entries with sequential timestamps
	entries := make([]entry, len(ids))
	for i, id := range ids {
		entries[i] = entry{id: id, updatedAtNs: int64(i * 1_000_000)}
	}

	// Apply cursor
	if cursor, ok := decodeHistoryCursor(pageToken); ok {
		startIdx := -1
		for i, e := range entries {
			if e.updatedAtNs == cursor.UpdatedAtNs && e.id == cursor.ID {
				startIdx = i + 1
				break
			}
		}
		if startIdx > 0 && startIdx < len(entries) {
			entries = entries[startIdx:]
		} else if startIdx >= len(entries) {
			entries = nil
		}
	}

	// Slice and build next token
	var nextPageToken string
	if pageSize > 0 && len(entries) > pageSize {
		last := entries[pageSize-1]
		nextPageToken = encodeHistoryCursor(historyCursor{
			UpdatedAtNs: last.updatedAtNs,
			ID:          last.id,
		})
		entries = entries[:pageSize]
	}

	result := paginationResult{nextPageToken: nextPageToken}
	for _, e := range entries {
		result.entries = append(result.entries, e.id)
	}
	return result
}

func TestPagination_SinglePage(t *testing.T) {
	ids := []string{"a", "b", "c"}
	r := simulatePage(ids, 10, "")
	if len(r.entries) != 3 {
		t.Errorf("got %d entries; want 3", len(r.entries))
	}
	if r.nextPageToken != "" {
		t.Errorf("expected empty next_page_token for single page; got %q", r.nextPageToken)
	}
}

func TestPagination_FirstPage(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	r := simulatePage(ids, 2, "")
	if len(r.entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(r.entries))
	}
	if r.entries[0] != "a" || r.entries[1] != "b" {
		t.Errorf("got %v; want [a b]", r.entries)
	}
	if r.nextPageToken == "" {
		t.Fatal("expected non-empty next_page_token")
	}
}

func TestPagination_SecondPage(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	r1 := simulatePage(ids, 2, "")
	r2 := simulatePage(ids, 2, r1.nextPageToken)
	if len(r2.entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(r2.entries))
	}
	if r2.entries[0] != "c" || r2.entries[1] != "d" {
		t.Errorf("got %v; want [c d]", r2.entries)
	}
	if r2.nextPageToken == "" {
		t.Fatal("expected non-empty next_page_token for second page")
	}
}

func TestPagination_LastPage(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	r1 := simulatePage(ids, 2, "")
	r2 := simulatePage(ids, 2, r1.nextPageToken)
	r3 := simulatePage(ids, 2, r2.nextPageToken)
	if len(r3.entries) != 1 {
		t.Fatalf("got %d entries; want 1", len(r3.entries))
	}
	if r3.entries[0] != "e" {
		t.Errorf("got %v; want [e]", r3.entries)
	}
	if r3.nextPageToken != "" {
		t.Errorf("expected empty next_page_token on last page; got %q", r3.nextPageToken)
	}
}

func TestPagination_FullTraversal(t *testing.T) {
	// Walk all pages and verify every entry appears exactly once.
	ids := make([]string, 25)
	for i := range ids {
		ids[i] = string(rune('A' + i))
	}

	var allSeen []string
	token := ""
	for {
		r := simulatePage(ids, 7, token)
		allSeen = append(allSeen, r.entries...)
		token = r.nextPageToken
		if token == "" {
			break
		}
	}

	if len(allSeen) != len(ids) {
		t.Fatalf("total entries seen across pages: %d; want %d", len(allSeen), len(ids))
	}
	for i, got := range allSeen {
		if got != ids[i] {
			t.Errorf("position %d: got %q; want %q", i, got, ids[i])
		}
	}
}

func TestPagination_StaleTokenReturnsFromBeginning(t *testing.T) {
	// If cursor references an entry not in the current list (e.g. cache refresh
	// removed it), the implementation falls back to returning from the beginning.
	ids := []string{"a", "b", "c"}
	// Create a cursor that points to an ID not in ids.
	staleToken := encodeHistoryCursor(historyCursor{UpdatedAtNs: 999, ID: "gone"})
	r := simulatePage(ids, 10, staleToken)
	if len(r.entries) != 3 {
		t.Errorf("got %d entries; want 3 (fallback to start)", len(r.entries))
	}
}

func TestPagination_EmptyList(t *testing.T) {
	r := simulatePage(nil, 10, "")
	if len(r.entries) != 0 {
		t.Errorf("got %d entries; want 0", len(r.entries))
	}
	if r.nextPageToken != "" {
		t.Errorf("expected empty next_page_token; got %q", r.nextPageToken)
	}
}

func TestPagination_PageSizeEqualsTotal(t *testing.T) {
	ids := []string{"a", "b", "c"}
	r := simulatePage(ids, 3, "")
	if len(r.entries) != 3 {
		t.Errorf("got %d entries; want 3", len(r.entries))
	}
	if r.nextPageToken != "" {
		t.Errorf("expected empty next_page_token; got %q", r.nextPageToken)
	}
}
