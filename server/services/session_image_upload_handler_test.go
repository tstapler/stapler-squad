package services

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// fakeInstanceStore is a minimal InstanceStore stub for upload handler tests.
type fakeInstanceStore struct {
	instances []*session.Instance
	loadErr   error
}

func (f *fakeInstanceStore) LoadInstances() ([]*session.Instance, error) {
	return f.instances, f.loadErr
}

func (f *fakeInstanceStore) ListInstanceData() ([]session.InstanceData, error) {
	return nil, nil
}

func (f *fakeInstanceStore) SaveInstances([]*session.Instance) error { return nil }

func (f *fakeInstanceStore) AddInstance(*session.Instance) error { return nil }

func (f *fakeInstanceStore) DeleteInstance(_ string) error { return nil }

func (f *fakeInstanceStore) UpdateInstanceLastUserResponse(_ string, _ time.Time) error { return nil }

// buildUploadRequest constructs a multipart/form-data POST request with the given
// session_id and file fields.
func buildUploadRequest(t *testing.T, sessionID, filename string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if sessionID != "" {
		if err := mw.WriteField("session_id", sessionID); err != nil {
			t.Fatalf("WriteField session_id: %v", err)
		}
	}
	if body != nil {
		part, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := part.Write(body); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/upload-image", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

// buildUploadRequestNoFile builds a multipart request with only a session_id field (no file).
func buildUploadRequestNoFile(t *testing.T, sessionID string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("session_id", sessionID); err != nil {
		t.Fatalf("WriteField session_id: %v", err)
	}
	mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/upload-image", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

// jpegMagic returns a minimal byte slice that looks like a JPEG.
func jpegMagic() []byte {
	return append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 508)...)
}

// pngMagic returns a minimal byte slice that looks like a PNG.
func pngMagic() []byte {
	return append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 504)...)
}

// webPMagic returns a minimal byte slice with RIFF+WEBP magic.
func webPMagic() []byte {
	b := make([]byte, 512)
	copy(b[0:4], []byte("RIFF"))
	copy(b[8:12], []byte("WEBP"))
	return b
}

func fixtureStore(t *testing.T) (*fakeInstanceStore, string) {
	t.Helper()
	dir := t.TempDir()
	inst := &session.Instance{}
	inst.ID = "session-123"
	inst.Title = "test-session"
	inst.Path = dir
	return &fakeInstanceStore{instances: []*session.Instance{inst}}, dir
}

// ── BT-01: JPEG success ──────────────────────────────────────────────────────

func TestSessionImageUpload_JPEG_Success(t *testing.T) {
	store, dir := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-123", "photo.jpg", jpegMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Path, filepath.Join(dir, "uploads")) {
		t.Errorf("path %q does not start with uploads dir", resp.Path)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Errorf("saved file does not exist: %v", err)
	}
	// Verify permissions.
	info, err := os.Stat(resp.Path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0o777 != sessionUploadFileMode {
		t.Errorf("file mode %o, want %o", info.Mode()&0o777, sessionUploadFileMode)
	}
}

// ── BT-02: PNG success ───────────────────────────────────────────────────────

func TestSessionImageUpload_PNG_Success(t *testing.T) {
	store, dir := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-123", "image.png", pngMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Path, filepath.Join(dir, "uploads")) {
		t.Errorf("unexpected path: %q", resp.Path)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Errorf("file not found: %v", err)
	}
}

// ── BT-03: WebP success ──────────────────────────────────────────────────────

func TestSessionImageUpload_WebP_Success(t *testing.T) {
	store, dir := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-123", "img.webp", webPMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Errorf("webp file not found: %v", err)
	}
	_ = dir
}

// ── BT-04: Oversized file → 413 ─────────────────────────────────────────────

func TestSessionImageUpload_OversizedFile(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	// Build a file clearly over the 10 MB file limit (MaxBytesReader adds 64 KB
	// overhead allowance for multipart boundaries/headers, so the file must
	// exceed 10 MB + 64 KB to trigger the 413).
	oversize := make([]byte, sessionUploadMaxBytes+128*1024)
	copy(oversize, jpegMagic())

	req := buildUploadRequest(t, "session-123", "big.jpg", oversize)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "too large") {
		t.Errorf("response does not mention 'too large': %q", rr.Body.String())
	}
}

// ── BT-05: Invalid MIME type → 400 ──────────────────────────────────────────

func TestSessionImageUpload_InvalidMIMEType(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-123", "page.html", []byte("<html>hello</html>"))
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "unsupported") {
		t.Errorf("response does not mention 'unsupported': %q", rr.Body.String())
	}
}

// ── BT-06: Empty file → 400 ──────────────────────────────────────────────────

func TestSessionImageUpload_EmptyFile(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-123", "empty.jpg", []byte{})
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ── BT-07: Session not found → 404 ──────────────────────────────────────────

func TestSessionImageUpload_SessionNotFound(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "nonexistent-id-xyz", "photo.jpg", jpegMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "not found") {
		t.Errorf("response does not mention 'not found': %q", rr.Body.String())
	}
}

// ── BT-08: Missing session_id → 400 ─────────────────────────────────────────

func TestSessionImageUpload_MissingSessionID(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	// Build a request with only the file field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", "photo.jpg")
	part.Write(jpegMagic()) //nolint:errcheck
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload-image", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "session_id is required") {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}

// ── BT-09: Missing file field → 400 ─────────────────────────────────────────

func TestSessionImageUpload_MissingFileField(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequestNoFile(t, "session-123")
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "file field required") {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}

// ── BT-10: Path traversal filename → sanitized, 200 ─────────────────────────

func TestSessionImageUpload_PathTraversalFilename(t *testing.T) {
	store, dir := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-123", "../../etc/passwd", jpegMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// The file must be inside the uploads dir (no traversal).
	if !strings.HasPrefix(resp.Path, filepath.Join(dir, "uploads")) {
		t.Errorf("path traversal! saved path %q not under uploads dir", resp.Path)
	}
	// The filename must not contain ".."
	if strings.Contains(resp.Filename, "..") {
		t.Errorf("filename %q contains '..', traversal not sanitized", resp.Filename)
	}
}

// ── BT-11: Session no path → 422 ─────────────────────────────────────────────

func TestSessionImageUpload_SessionNoPath(t *testing.T) {
	inst := &session.Instance{}
	inst.ID = "session-nopath"
	inst.Title = "no-path-session"
	inst.Path = ""
	store := &fakeInstanceStore{instances: []*session.Instance{inst}}
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-nopath", "photo.jpg", jpegMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "no working directory") {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}

// ── BT-12: Session path missing on disk → 422 ───────────────────────────────

func TestSessionImageUpload_SessionPathMissingOnDisk(t *testing.T) {
	inst := &session.Instance{}
	inst.ID = "session-badpath"
	inst.Title = "bad-path-session"
	inst.Path = "/tmp/nonexistent-stapler-test-dir-xyz"
	store := &fakeInstanceStore{instances: []*session.Instance{inst}}
	h := NewSessionImageUploadHandler(store)

	req := buildUploadRequest(t, "session-badpath", "photo.jpg", jpegMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(strings.ToLower(rr.Body.String()), "not accessible") {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}

// ── BT-13: Concurrent uploads produce unique filenames ───────────────────────

func TestSessionImageUpload_ConcurrentUploads(t *testing.T) {
	store, _ := fixtureStore(t)
	h := NewSessionImageUploadHandler(store)

	// Build requests before spawning goroutines — t.Fatal is not safe inside goroutines.
	reqs := []*http.Request{
		buildUploadRequest(t, "session-123", "photo.jpg", jpegMagic()),
		buildUploadRequest(t, "session-123", "photo.jpg", jpegMagic()),
	}

	type result struct {
		code int
		path string
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rr := httptest.NewRecorder()
			h.HandleUpload(rr, reqs[idx])
			results[idx].code = rr.Code
			if rr.Code == http.StatusOK {
				var resp sessionImageUploadResponse
				if err := json.NewDecoder(rr.Body).Decode(&resp); err == nil {
					results[idx].path = resp.Path
				}
			}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r.code != http.StatusOK {
			t.Errorf("goroutine %d: expected 200, got %d", i, r.code)
		}
	}
	if results[0].path == results[1].path {
		t.Errorf("concurrent uploads produced identical paths: %q", results[0].path)
	}
	// Both files must exist.
	for _, r := range results {
		if _, err := os.Stat(r.path); err != nil {
			t.Errorf("file not found: %s: %v", r.path, err)
		}
	}
}

// ── Additional: detectWebP helper ────────────────────────────────────────────

func TestDetectWebP(t *testing.T) {
	if !detectWebP(webPMagic()) {
		t.Error("detectWebP returned false for valid WebP magic bytes")
	}
	if detectWebP(jpegMagic()) {
		t.Error("detectWebP returned true for JPEG bytes")
	}
	if detectWebP([]byte{}) {
		t.Error("detectWebP returned true for empty bytes")
	}
}

// ── Additional: sanitizeFilename helper ──────────────────────────────────────

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"photo.jpg", "photo.jpg"},
		{"../../etc/passwd", "passwd"},
		{"/absolute/path/file.png", "file.png"},
		{"", "upload"},
		{".", "upload"},
		{"..", "upload"},
		{strings.Repeat("a", 200) + ".jpg", strings.Repeat("a", 96) + ".jpg"},
	}
	for _, tc := range tests {
		got := sanitizeFilename(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
