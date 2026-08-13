package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

func TestScanMarkers_HandlerFile(t *testing.T) {
	markers, err := ScanMarkers(testdataDir())
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}

	entry, ok := markers["session:create"]
	if !ok {
		t.Fatalf("expected 'session:create' marker, got keys: %v", markerKeys(markers))
	}
	if entry.FeatureID != "session:create" {
		t.Errorf("expected FeatureID=session:create, got %q", entry.FeatureID)
	}
	if entry.FilePath == "" {
		t.Error("expected FilePath to be set")
	}
}

func TestScanMarkers_ExcludesPbGo(t *testing.T) {
	markers, err := ScanMarkers(testdataDir())
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}

	// handler.pb.go has a function but any // +api: comment in it should be excluded.
	// The testdata pb.go does not contain +api: markers, but we verify the .pb.go file
	// itself is skipped by checking it doesn't accidentally add entries from that file.
	for _, e := range markers {
		base := filepath.Base(e.FilePath)
		if filepath.Ext(e.FilePath) == ".go" && len(base) > 6 && base[len(base)-6:] == ".pb.go" {
			t.Errorf("found marker entry from .pb.go file: %s", e.FilePath)
		}
	}
}

func TestScanMarkers_ExcludesTestGo(t *testing.T) {
	markers, err := ScanMarkers(testdataDir())
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}

	for _, e := range markers {
		base := filepath.Base(e.FilePath)
		if len(base) > 8 && base[len(base)-8:] == "_test.go" {
			t.Errorf("found marker entry from _test.go file: %s", e.FilePath)
		}
	}
}

func TestScanMarkers_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	markers, err := ScanMarkers(tmp)
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}
	if len(markers) != 0 {
		t.Fatalf("expected 0 markers for empty dir, got %d", len(markers))
	}
}

func TestScanMarkers_NoMarkersInPbGo(t *testing.T) {
	// Verify that even if we inject +api: into a .pb.go file in a temp dir, it's excluded.
	tmp := t.TempDir()
	pbPath := filepath.Join(tmp, "session.pb.go")
	content := `package services

// +api: session:pb-ghost

func PbFunc() {}
`
	if err := os.WriteFile(pbPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	markers, err := ScanMarkers(tmp)
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}
	if _, ok := markers["session:pb-ghost"]; ok {
		t.Error("marker from .pb.go file should be excluded")
	}
}

func TestScanMarkers_NoMarkersInTestGo(t *testing.T) {
	tmp := t.TempDir()
	testPath := filepath.Join(tmp, "session_test.go")
	content := `package services

// +api: session:test-ghost

func TestFoo(t *testing.T) {}
`
	if err := os.WriteFile(testPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	markers, err := ScanMarkers(tmp)
	if err != nil {
		t.Fatalf("ScanMarkers error: %v", err)
	}
	if _, ok := markers["session:test-ghost"]; ok {
		t.Error("marker from _test.go file should be excluded")
	}
}

func markerKeys(m map[string]MarkerEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
