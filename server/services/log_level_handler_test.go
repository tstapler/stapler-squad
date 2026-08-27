package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tstapler/stapler-squad/log"
)

func postPackageLevel(t *testing.T, h *LogLevelHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/debug/log-level/packages", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.HandleSetPackage(rec, req)
	return rec
}

// TestHandleSetPackage_RejectsRaisingAuditSensitivePackageAboveWarning guards
// against silencing server/auth's Info/Warn audit trail (login, passkey
// registration, session invalidation) via the more surgical per-package
// debug endpoint — the pre-existing global level toggle is coarser and not
// restricted the same way.
func TestHandleSetPackage_RejectsRaisingAuditSensitivePackageAboveWarning(t *testing.T) {
	t.Cleanup(log.ClearAllPackageLevels)
	h := NewLogLevelHandler()

	rec := postPackageLevel(t, h, `{"package":"server/auth","level":"ERROR"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	overrides := log.GetPackageLevels()
	if _, ok := overrides["server/auth"]; ok {
		t.Fatalf("expected no override to be applied, got %v", overrides)
	}
}

// TestHandleSetPackage_RejectsRaisingAuditSensitiveSubpackageAboveWarning
// verifies the guard also covers subpackages of an audit-sensitive package.
func TestHandleSetPackage_RejectsRaisingAuditSensitiveSubpackageAboveWarning(t *testing.T) {
	t.Cleanup(log.ClearAllPackageLevels)
	h := NewLogLevelHandler()

	rec := postPackageLevel(t, h, `{"package":"server/auth/passkey","level":"FATAL"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

// TestHandleSetPackage_AllowsAuditSensitivePackageAtOrBelowWarning verifies
// the guard only blocks raising the level above WARNING — WARNING itself and
// more-verbose levels (which don't suppress the audit trail) are still
// allowed through the same endpoint.
func TestHandleSetPackage_AllowsAuditSensitivePackageAtOrBelowWarning(t *testing.T) {
	t.Cleanup(log.ClearAllPackageLevels)
	h := NewLogLevelHandler()

	rec := postPackageLevel(t, h, `{"package":"server/auth","level":"DEBUG"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	overrides := log.GetPackageLevels()
	if lvl, ok := overrides["server/auth"]; !ok || lvl != log.DEBUG {
		t.Fatalf("expected server/auth=DEBUG override, got %v", overrides)
	}
}

// TestHandleSetPackage_RejectsUnrecognizedLevel verifies an unrecognized
// level string is rejected with 400 rather than silently coerced to INFO
// (log.ParseLogLevel's default for unknown input).
func TestHandleSetPackage_RejectsUnrecognizedLevel(t *testing.T) {
	t.Cleanup(log.ClearAllPackageLevels)
	h := NewLogLevelHandler()

	rec := postPackageLevel(t, h, `{"package":"session/tmux","level":"VERBOSE"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	if overrides := log.GetPackageLevels(); len(overrides) != 0 {
		t.Fatalf("expected no override applied for an invalid level, got %v", overrides)
	}
}

// TestHandleGetPackages_SortsEntriesByPackage guards against nondeterministic
// map-iteration order leaking into the JSON response.
func TestHandleGetPackages_SortsEntriesByPackage(t *testing.T) {
	t.Cleanup(log.ClearAllPackageLevels)
	h := NewLogLevelHandler()

	log.SetPackageLevel("session/tmux", log.DEBUG)
	log.SetPackageLevel("config", log.WARNING)
	log.SetPackageLevel("session", log.ERROR)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/log-level/packages", nil)
	rec := httptest.NewRecorder()
	h.HandleGetPackages(rec, req)

	var entries []packageLevelEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Package > entries[i].Package {
			t.Fatalf("entries not sorted by package: %v", entries)
		}
	}
}

// TestHandleSetPackage_RejectsOversizedBody verifies the new handlers are
// bounded by http.MaxBytesReader like the rest of the app's request bodies.
func TestHandleSetPackage_RejectsOversizedBody(t *testing.T) {
	t.Cleanup(log.ClearAllPackageLevels)
	h := NewLogLevelHandler()

	oversized := `{"package":"session/tmux","level":"` + string(make([]byte, maxPackageLevelBodyBytes)) + `"}`
	rec := postPackageLevel(t, h, oversized)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
