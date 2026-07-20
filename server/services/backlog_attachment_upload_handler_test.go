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
	"testing"
)

// gifMagic returns a minimal byte slice that looks like a GIF89a image.
func gifMagic() []byte {
	return append([]byte("GIF89a"), make([]byte, 506)...)
}

// backlogWebPMagic returns a minimal byte slice with a full RIFF/WEBP magic
// header. Unlike the webPMagic() helper in session_image_upload_handler_test.go
// (which only sets "WEBP" at offset 8 and is never exercised through strict
// sniffing, since the session upload handler accepts any file type), this
// handler validates via http.DetectContentType, whose WEBP signature requires
// "WEBPVP" at offset 8 — so the full 6-byte marker is needed here.
func backlogWebPMagic() []byte {
	b := make([]byte, 512)
	copy(b[0:4], []byte("RIFF"))
	copy(b[8:14], []byte("WEBPVP"))
	return b
}

func buildBacklogAttachmentRequest(t *testing.T, filename string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
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
	r := httptest.NewRequest(http.MethodPost, "/api/v1/upload-backlog-attachment", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func TestBacklogAttachmentUpload_PNG_Success(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "screenshot.png", pngMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp backlogAttachmentUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Path, dir) {
		t.Errorf("path %q not under attachment dir %q", resp.Path, dir)
	}
	if !strings.HasSuffix(resp.Path, ".png") {
		t.Errorf("expected saved path to end in .png, got %q", resp.Path)
	}
	if _, err := os.Stat(resp.Path); err != nil {
		t.Errorf("saved file not found: %v", err)
	}
}

func TestBacklogAttachmentUpload_JPEG_Success(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "photo.jpg", jpegMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBacklogAttachmentUpload_GIF_Success(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "anim.gif", gifMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp backlogAttachmentUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasSuffix(resp.Path, ".gif") {
		t.Errorf("expected saved path to end in .gif, got %q", resp.Path)
	}
}

func TestBacklogAttachmentUpload_WebP_Success(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "photo.webp", backlogWebPMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp backlogAttachmentUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasSuffix(resp.Path, ".webp") {
		t.Errorf("expected saved path to end in .webp, got %q", resp.Path)
	}
}

func TestBacklogAttachmentUpload_SpoofedExtension_Rejected(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	// Named like an image but the content is plain text — magic-byte sniffing
	// must catch this even though the extension/declared type looks legit.
	req := buildBacklogAttachmentRequest(t, "totally-a-photo.png", []byte("<html>not an image</html>"))
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBacklogAttachmentUpload_SVG_Rejected(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	req := buildBacklogAttachmentRequest(t, "logo.svg", svg)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for SVG upload, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBacklogAttachmentUpload_PathTraversalFilename(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "../../etc/passwd.png", pngMagic())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp backlogAttachmentUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if filepath.Dir(resp.Path) != filepath.Clean(dir) {
		t.Errorf("saved file escaped attachment dir: %q", resp.Path)
	}
}

func TestBacklogAttachmentUpload_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	oversize := make([]byte, backlogAttachmentMaxBytes+128*1024)
	copy(oversize, pngMagic())

	req := buildBacklogAttachmentRequest(t, "big.png", oversize)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBacklogAttachmentUpload_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "empty.png", []byte{})
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBacklogAttachmentUpload_MissingFileField(t *testing.T) {
	dir := t.TempDir()
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatalf("NewBacklogAttachmentUploadHandler: %v", err)
	}

	req := buildBacklogAttachmentRequest(t, "", nil)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
