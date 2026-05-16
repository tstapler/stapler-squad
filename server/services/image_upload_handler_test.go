package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newFileUploadHandler(t *testing.T) (*FileUploadHandler, string) {
	t.Helper()
	dir := t.TempDir()
	return NewFileUploadHandler(dir), dir
}

func postJSON(t *testing.T, h *FileUploadHandler, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload/file", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)
	return rr
}

func TestHandleUpload_ValidPNG(t *testing.T) {
	h, dir := newFileUploadHandler(t)

	payload := []byte("fake-png-data")
	encoded := base64.StdEncoding.EncodeToString(payload)

	rr := postJSON(t, h, map[string]string{"data": encoded, "contentType": "image/png"})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp fileUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasSuffix(resp.Path, ".png") || !strings.Contains(resp.Path, "paste-") {
		t.Errorf("expected paste-*.png path, got %q", resp.Path)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Errorf("saved file not found: %v", err)
	}

	info, _ := os.Stat(resp.Path)
	if info.Mode().Perm() != uploadFileMode {
		t.Errorf("expected file mode %o, got %o", uploadFileMode, info.Mode().Perm())
	}

	got, _ := os.ReadFile(resp.Path)
	if !bytes.Equal(got, payload) {
		t.Errorf("file content mismatch")
	}
	_ = dir
}

func TestHandleUpload_WrongMethod(t *testing.T) {
	h, _ := newFileUploadHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/upload/file", nil)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleUpload_InvalidBase64(t *testing.T) {
	h, _ := newFileUploadHandler(t)
	rr := postJSON(t, h, map[string]string{"data": "not-valid-base64!!!", "contentType": "image/png"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleUpload_InvalidJSON(t *testing.T) {
	h, _ := newFileUploadHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/upload/file", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestExtensionForMIME(t *testing.T) {
	cases := []struct {
		name        string
		ct          string
		originalExt string
		want        string
	}{
		{"png", "image/png", "", ".png"},
		{"jpeg", "image/jpeg", "", ".jpg"},
		{"gif", "image/gif", "", ".gif"},
		{"webp", "image/webp", "", ".webp"},
		{"bmp", "image/bmp", "", ".bmp"},
		{"json", "application/json", "", ".json"},
		{"zip", "application/zip", "", ".zip"},
		{"python", "text/x-python", "", ".py"},
		{"octet_stream_no_fallback", "application/octet-stream", "", ".bin"},
		{"octet_stream_table_wins", "application/octet-stream", ".go", ".bin"},
		{"unknown_with_fallback", "application/x-unknown", ".rs", ".rs"},
		{"unknown_no_fallback", "application/x-unknown", "", ".bin"},
		{"params_stripped", "image/png; charset=utf-8", "", ".png"},
		{"uppercase", "IMAGE/PNG", "", ".png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extensionForMIME(tc.ct, tc.originalExt)
			if got != tc.want {
				t.Errorf("extensionForMIME(%q, %q) = %q, want %q", tc.ct, tc.originalExt, got, tc.want)
			}
		})
	}
}

func TestSanitizeExtension(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"report.pdf", ".pdf"},
		{"archive.tar.gz", ".gz"},
		{"../../etc/passwd", ""},   // base="passwd" has no extension → safe empty return
		{"file", ""},
		{"file.", ""},
		{"file.UPPERCASE", ".uppercase"},
		{"file.very-long-ext-here", ""},
		{"", ""},
		{"has spaces .txt", ".txt"},
		{"../../../.bashrc", ".bashrc"},
	}
	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			got := sanitizeExtension(tc.filename)
			if got != tc.want {
				t.Errorf("sanitizeExtension(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func TestCleanOldPasteFiles(t *testing.T) {
	dir := t.TempDir()

	// Write an old file (mtime in the past)
	old := filepath.Join(dir, "paste-old.png")
	if err := os.WriteFile(old, []byte("old"), uploadFileMode); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-maxPasteFileAge - time.Second)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	// Write a recent file
	recent := filepath.Join(dir, "paste-recent.png")
	if err := os.WriteFile(recent, []byte("new"), uploadFileMode); err != nil {
		t.Fatal(err)
	}

	cleanOldPasteFiles(dir)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("expected old file to be removed")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("expected recent file to be kept")
	}
}

// TestHandleUpload_NonImageMIMEAccepted verifies that non-image MIME types are accepted.
func TestHandleUpload_NonImageMIMEAccepted(t *testing.T) {
	h, _ := newFileUploadHandler(t)
	cases := []struct {
		ct      string
		wantExt string
	}{
		{"application/json", ".json"},
		{"application/pdf", ".pdf"},
		{"application/zip", ".zip"},
		{"text/x-python", ".py"},
		{"application/octet-stream", ".bin"},
	}
	for _, tc := range cases {
		t.Run(tc.ct, func(t *testing.T) {
			rr := postJSON(t, h, map[string]string{
				"data":        base64.StdEncoding.EncodeToString([]byte("fake")),
				"contentType": tc.ct,
			})
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			var resp fileUploadResponse
			_ = json.NewDecoder(rr.Body).Decode(&resp)
			if !strings.HasSuffix(resp.Path, tc.wantExt) {
				t.Errorf("expected path ending in %q, got %q", tc.wantExt, resp.Path)
			}
		})
	}
}

// TestHandleUpload_OriginalFilenameExtFallback verifies extension fallback via filename.
func TestHandleUpload_OriginalFilenameExtFallback(t *testing.T) {
	h, _ := newFileUploadHandler(t)
	rr := postJSON(t, h, map[string]string{
		"data":             base64.StdEncoding.EncodeToString([]byte("fake")),
		"contentType":      "application/x-unknown-type",
		"originalFilename": "my_script.rs",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp fileUploadResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if !strings.HasSuffix(resp.Path, ".rs") {
		t.Errorf("expected .rs extension, got %q", resp.Path)
	}
}

// TestHandleUpload_DecodedSizeLimitEnforced verifies that a file that decodes to
// more than maxUploadBytes is rejected even if the encoded body fits in MaxBytesReader.
func TestHandleUpload_DecodedSizeLimitEnforced(t *testing.T) {
	h, _ := newFileUploadHandler(t)
	// Construct decoded data just over the limit
	oversize := make([]byte, maxUploadBytes+1)
	encoded := base64.StdEncoding.EncodeToString(oversize)
	rr := postJSON(t, h, map[string]string{
		"data":        encoded,
		"contentType": "application/octet-stream",
	})
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestHandleUpload_PathTraversalInFilename verifies that a crafted originalFilename
// cannot escape the paste directory.
func TestHandleUpload_PathTraversalInFilename(t *testing.T) {
	h, dir := newFileUploadHandler(t)
	rr := postJSON(t, h, map[string]string{
		"data":             base64.StdEncoding.EncodeToString([]byte("fake")),
		"contentType":      "application/x-unknown-type",
		"originalFilename": "../../etc/passwd",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp fileUploadResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	// Path must be inside the paste dir
	if !strings.HasPrefix(resp.Path, dir) {
		t.Errorf("path escaped paste dir: %q (expected prefix %q)", resp.Path, dir)
	}
}
