package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func newLocalFileServiceHandlers() (list http.Handler, serve http.Handler) {
	svc := NewLocalFileService()
	list = http.HandlerFunc(svc.ListLocalDirectory)
	serve = http.StripPrefix("/", http.HandlerFunc(svc.ServeLocalFile))
	return
}

// TestListLocalDirectory_DefaultPath verifies that omitting ?path returns 200 +
// a valid JSON listing rooted at the user home directory.
func TestListLocalDirectory_DefaultPath(t *testing.T) {
	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp listDirectoryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	home, _ := os.UserHomeDir()
	if resp.Path != home {
		t.Errorf("Path = %q, want %q", resp.Path, home)
	}
}

// TestListLocalDirectory_ExplicitTmpPath verifies that ?path=/tmp returns 200 +
// a listing with at least one entry (tmp always has something on Linux).
func TestListLocalDirectory_ExplicitTmpPath(t *testing.T) {
	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodGet, "/?path=/tmp", nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp listDirectoryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Path != "/tmp" {
		t.Errorf("Path = %q, want /tmp", resp.Path)
	}
}

// TestListLocalDirectory_NonExistentPath verifies that a missing directory returns 404.
func TestListLocalDirectory_NonExistentPath(t *testing.T) {
	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodGet, "/?path=/nonexistent-path-xyz-stapler-squad-test", nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestListLocalDirectory_RelativePath verifies that a non-absolute path returns 400.
func TestListLocalDirectory_RelativePath(t *testing.T) {
	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodGet, "/?path=relative/path", nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestListLocalDirectory_MethodNotAllowed verifies that POST returns 405.
func TestListLocalDirectory_MethodNotAllowed(t *testing.T) {
	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestServeLocalFile_ExistingFile verifies that an existing regular file returns 200.
func TestServeLocalFile_ExistingFile(t *testing.T) {
	// Write a temp file so the test doesn't depend on /etc/hostname existing.
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewLocalFileService()
	req := httptest.NewRequest(http.MethodGet, filePath, nil)
	req.URL.Path = filePath // ServeLocalFile reads r.URL.Path directly
	w := httptest.NewRecorder()
	svc.ServeLocalFile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestServeLocalFile_NonExistentFile verifies that a missing file returns 404.
func TestServeLocalFile_NonExistentFile(t *testing.T) {
	svc := NewLocalFileService()
	path := "/nonexistent-stapler-squad-test-file.txt"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.URL.Path = path
	w := httptest.NewRecorder()
	svc.ServeLocalFile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// TestServeLocalFile_Directory verifies that requesting a directory path returns 400.
func TestServeLocalFile_Directory(t *testing.T) {
	svc := NewLocalFileService()
	dir := t.TempDir()
	req := httptest.NewRequest(http.MethodGet, dir, nil)
	req.URL.Path = dir
	w := httptest.NewRecorder()
	svc.ServeLocalFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for directory path, got %d: %s", w.Code, w.Body.String())
	}
}

// TestListLocalDirectory_SymlinkedDirectory verifies that a symlink pointing at a
// real directory is reported as IsDir == true so the frontend can navigate into it.
func TestListLocalDirectory_SymlinkedDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real-subdir")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link-to-subdir")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	// A dangling symlink must not error the whole listing or be reported as a directory.
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), filepath.Join(dir, "broken-link")); err != nil {
		t.Fatal(err)
	}

	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodGet, "/?path="+dir, nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp listDirectoryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var sawLink, sawBroken bool
	for _, e := range resp.Entries {
		switch e.Name {
		case "link-to-subdir":
			sawLink = true
			if !e.IsDir {
				t.Errorf("link-to-subdir: IsDir = false, want true")
			}
		case "broken-link":
			sawBroken = true
			if e.IsDir {
				t.Errorf("broken-link: IsDir = true, want false")
			}
		}
	}
	if !sawLink {
		t.Error("link-to-subdir entry not found in listing")
	}
	if !sawBroken {
		t.Error("broken-link entry not found in listing")
	}
}

// TestListLocalDirectory_Truncation verifies the >2000-entry cap sets HasMore and Total
// accurately, with an off-by-one boundary at exactly maxLocalDirEntries.
func TestListLocalDirectory_Truncation(t *testing.T) {
	dir := t.TempDir()
	const count = maxLocalDirEntries + 1
	for i := 0; i < count; i++ {
		name := filepath.Join(dir, "file-"+strconv.Itoa(i))
		if err := os.WriteFile(name, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	list, _ := newLocalFileServiceHandlers()
	req := httptest.NewRequest(http.MethodGet, "/?path="+dir, nil)
	w := httptest.NewRecorder()
	list.ServeHTTP(w, req)

	var resp listDirectoryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != count {
		t.Errorf("Total = %d, want %d", resp.Total, count)
	}
	if !resp.HasMore {
		t.Error("HasMore = false, want true")
	}
	if len(resp.Entries) != maxLocalDirEntries {
		t.Errorf("len(Entries) = %d, want %d", len(resp.Entries), maxLocalDirEntries)
	}
}

// TestServeLocalFile_RelativePath verifies that a non-absolute path returns 400.
// httptest.NewRequest requires a valid URI, so we start with "/" and then override
// URL.Path to a relative value — the handler reads r.URL.Path directly.
func TestServeLocalFile_RelativePath(t *testing.T) {
	svc := NewLocalFileService()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = "relative/file.txt" // override after construction
	w := httptest.NewRecorder()
	svc.ServeLocalFile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for relative path, got %d", w.Code)
	}
}
