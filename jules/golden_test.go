package jules

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// readFixture reads a file from jules/testdata (see testdata/README.md for
// provenance).
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func TestGoldenFixtures_should_DecodeIntoJulesSession_When_SessionCompleted(t *testing.T) {
	var session JulesSession
	if err := json.Unmarshal(readFixture(t, "session_completed.json"), &session); err != nil {
		t.Fatalf("decoding session_completed.json: %v", err)
	}
	if session.State != JulesStateCompleted {
		t.Fatalf("State = %v, want JulesStateCompleted", session.State)
	}
	if len(session.Outputs) == 0 || session.Outputs[0].PullRequest == nil || session.Outputs[0].PullRequest.URL == "" {
		t.Fatalf("Outputs = %+v, want a non-empty PullRequest.URL", session.Outputs)
	}
}

func TestGoldenFixtures_should_DecodeIntoJulesSession_When_SessionCreated(t *testing.T) {
	var session JulesSession
	if err := json.Unmarshal(readFixture(t, "session_created.json"), &session); err != nil {
		t.Fatalf("decoding session_created.json: %v", err)
	}
	if session.State != JulesStateQueued {
		t.Fatalf("State = %v, want JulesStateQueued", session.State)
	}
	if session.Name == "" {
		t.Fatal("Name is empty")
	}
}

func TestGoldenFixtures_should_DecodeIntoJulesSession_When_SessionFailed(t *testing.T) {
	var session JulesSession
	if err := json.Unmarshal(readFixture(t, "session_failed.json"), &session); err != nil {
		t.Fatalf("decoding session_failed.json: %v", err)
	}
	if session.State != JulesStateFailed {
		t.Fatalf("State = %v, want JulesStateFailed", session.State)
	}
	if !session.State.IsTerminal() {
		t.Fatal("expected FAILED to be terminal")
	}
}

func TestGoldenFixtures_should_DecodeIntoJulesSources_When_SourcesListed(t *testing.T) {
	var body julesListSourcesResponse
	if err := json.Unmarshal(readFixture(t, "sources_list.json"), &body); err != nil {
		t.Fatalf("decoding sources_list.json: %v", err)
	}
	if len(body.Sources) == 0 {
		t.Fatal("expected at least one source")
	}
	for _, src := range body.Sources {
		if _, err := ParseJulesSourceName(string(src.Name)); err != nil {
			t.Errorf("source %+v has an invalid name: %v", src, err)
		}
	}
}

// TestGoldenFixtures_should_RejectUnknownFields_When_SchemaDrifts demonstrates
// encoding/json's DisallowUnknownFields behavior in isolation, using a local
// decoder this test builds itself. Production decoding in client.go does NOT
// enable DisallowUnknownFields (verified: grep -rn DisallowUnknownFields
// jules/ finds only this test) -- an unrecognized field from a Jules API
// schema change is silently dropped there, not rejected. That is a
// deliberate forward-compatibility choice for an evolving alpha API, not a
// gap, but this test does not prove anything about production behavior.
func TestGoldenFixtures_should_RejectUnknownFields_When_SchemaDrifts(t *testing.T) {
	drifted := []byte(`{"name":"sessions/abc","state":"COMPLETED","unexpectedNewField":"surprise"}`)

	dec := json.NewDecoder(bytes.NewReader(drifted))
	dec.DisallowUnknownFields()
	var session JulesSession
	err := dec.Decode(&session)

	if err == nil {
		t.Fatal("expected a decode error for the unrecognized field, got nil")
	}
	if !strings.Contains(err.Error(), "unexpectedNewField") {
		t.Fatalf("expected error to name the unexpected field, got %q", err.Error())
	}
}
