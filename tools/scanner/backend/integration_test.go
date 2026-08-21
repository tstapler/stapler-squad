package backend

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIntegration_ProtoAndMarkersMerge verifies that proto scan + marker scan
// correctly merges to produce MarkerFound=true for a matched feature.
func TestIntegration_ProtoAndMarkersMerge(t *testing.T) {
	tmp := t.TempDir()

	// Create a minimal proto file.
	protoContent := `syntax = "proto3";
package session.v1;

service SessionService {
  rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse) {}
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse) {}
}
`
	protoPath := filepath.Join(tmp, "session.proto")
	if err := os.WriteFile(protoPath, []byte(protoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a handler file with the marker for CreateSession.
	handlerContent := `package services

// +api: session:create
func (s *SessionService) CreateSession() {}
`
	handlerPath := filepath.Join(tmp, "session_service.go")
	if err := os.WriteFile(handlerPath, []byte(handlerContent), 0644); err != nil {
		t.Fatal(err)
	}

	features, err := ScanProto(protoPath)
	if err != nil {
		t.Fatalf("ScanProto error: %v", err)
	}
	if len(features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(features))
	}

	markers, err := ScanMarkers(tmp)
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}

	// Verify session:create has a marker and session:list does not.
	var createFound, listFound bool
	for _, f := range features {
		if f.ID == "session:create" {
			createFound = true
			m, ok := markers[f.ID]
			if !ok {
				t.Error("expected marker for session:create")
			} else if m.FilePath != handlerPath {
				t.Errorf("expected HandlerFile=%s, got %s", handlerPath, m.FilePath)
			}
		}
		if f.ID == "session:list" {
			listFound = true
			if _, ok := markers[f.ID]; ok {
				t.Error("unexpected marker for session:list")
			}
		}
	}
	if !createFound {
		t.Error("session:create feature not found")
	}
	if !listFound {
		t.Error("session:list feature not found")
	}
}
