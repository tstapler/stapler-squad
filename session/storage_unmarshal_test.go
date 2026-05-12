package session

import (
	"encoding/json"
	"testing"
)

func TestClaudeSessionData_UnmarshalJSON_LegacyConversationID(t *testing.T) {
	// Persisted state written before SquadSessionID was renamed from
	// ConversationID. The legacy "conversation_id" key must map onto the
	// new SquadSessionID field so existing sessions hydrate correctly.
	payload := []byte(`{"session_id":"sess-1","conversation_id":"legacy-id","project_name":"demo"}`)

	var got ClaudeSessionData
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ConversationUUID != "sess-1" {
		t.Errorf("ConversationUUID = %q, want %q", got.ConversationUUID, "sess-1")
	}
	if got.SquadSessionID != "legacy-id" {
		t.Errorf("SquadSessionID = %q, want %q (legacy conversation_id fallback)", got.SquadSessionID, "legacy-id")
	}
	if got.ProjectName != "demo" {
		t.Errorf("ProjectName = %q, want %q", got.ProjectName, "demo")
	}
}

func TestClaudeSessionData_UnmarshalJSON_NewKeyTakesPrecedence(t *testing.T) {
	// When both keys are present (transitional state), the new
	// squad_session_id key wins over the legacy conversation_id key.
	payload := []byte(`{"squad_session_id":"new-id","conversation_id":"old-id"}`)

	var got ClaudeSessionData
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SquadSessionID != "new-id" {
		t.Errorf("SquadSessionID = %q, want %q", got.SquadSessionID, "new-id")
	}
}

func TestClaudeSessionData_UnmarshalJSON_NewFormatOnly(t *testing.T) {
	// Newly written state has no conversation_id key; the new
	// squad_session_id key is used directly.
	payload := []byte(`{"session_id":"sess-2","squad_session_id":"squad-2"}`)

	var got ClaudeSessionData
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SquadSessionID != "squad-2" {
		t.Errorf("SquadSessionID = %q, want %q", got.SquadSessionID, "squad-2")
	}
	if got.ConversationUUID != "sess-2" {
		t.Errorf("ConversationUUID = %q, want %q", got.ConversationUUID, "sess-2")
	}
}
