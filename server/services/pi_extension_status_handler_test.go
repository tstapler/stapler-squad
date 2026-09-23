package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHandlePiExtensionStatus_ShouldReportFalse_WhenExtensionFileAbsent covers
// Story 2.1.2's "no warning" branch: with nothing installed at
// ~/.pi/agent/extensions/ssq-approval.ts, the endpoint reports installed=false.
func TestHandlePiExtensionStatus_ShouldReportFalse_WhenExtensionFileAbsent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	svc := NewPiExtensionStatusService()
	req := httptest.NewRequest(http.MethodGet, "/api/pi-extension-status", nil)
	w := httptest.NewRecorder()
	svc.HandlePiExtensionStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp piExtensionStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Installed {
		t.Errorf("Installed = true, want false when the extension file does not exist")
	}
}

// TestHandlePiExtensionStatus_ShouldReportTrue_WhenExtensionFilePresent covers
// Story 2.1.2's "mandatory warning" branch: with ssq-approval.ts present on
// disk, the endpoint reports installed=true.
func TestHandlePiExtensionStatus_ShouldReportTrue_WhenExtensionFilePresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	extDir := filepath.Join(home, ".pi", "agent", "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "ssq-approval.ts"), []byte("// stub"), 0o644); err != nil {
		t.Fatalf("write extension stub: %v", err)
	}

	svc := NewPiExtensionStatusService()
	req := httptest.NewRequest(http.MethodGet, "/api/pi-extension-status", nil)
	w := httptest.NewRecorder()
	svc.HandlePiExtensionStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp piExtensionStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Installed {
		t.Errorf("Installed = false, want true when the extension file exists")
	}
}

// TestHandlePiExtensionStatus_ShouldRejectNonGet_WhenMethodIsNotGet is a
// regression guard matching the sibling local-file handlers' method check.
func TestHandlePiExtensionStatus_ShouldRejectNonGet_WhenMethodIsNotGet(t *testing.T) {
	svc := NewPiExtensionStatusService()
	req := httptest.NewRequest(http.MethodPost, "/api/pi-extension-status", nil)
	w := httptest.NewRecorder()
	svc.HandlePiExtensionStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
