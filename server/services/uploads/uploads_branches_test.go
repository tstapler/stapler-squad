package uploads

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
)

type stubFinder struct {
	inst  *instance.Instance
	calls int
}

func (s *stubFinder) FindInstance(string) *instance.Instance {
	s.calls++
	return s.inst
}

func rawMultipartRequest(t *testing.T, target string, body []byte, contentType string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	return r
}

func assertBody(t *testing.T, rr *httptest.ResponseRecorder, code int, body string) {
	t.Helper()
	if rr.Code != code || rr.Body.String() != body {
		t.Fatalf("got %d %q, want %d %q", rr.Code, rr.Body.String(), code, body)
	}
}

func TestSessionImageUpload_WrongMethod_405(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	assertBody(t, rr, http.StatusMethodNotAllowed, "method not allowed\n")
}

func TestSessionImageUpload_BadMultipart_400(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	rr := httptest.NewRecorder()
	req := rawMultipartRequest(t, "/x", []byte("not multipart"), "text/plain")
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, req)
	assertBody(t, rr, http.StatusBadRequest, "bad multipart form\n")
}

func TestSessionImageUpload_BodyOverCap_413(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	rr := httptest.NewRecorder()
	big := bytes.Repeat([]byte{0x89}, sessionUploadMaxBytes+128*1024)
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "big.bin", big))
	assertBody(t, rr, http.StatusRequestEntityTooLarge, "file too large (max 10 MB)\n")
}

func TestSessionImageUpload_FinderHit_SkipsStorage(t *testing.T) {
	t.Parallel()
	store, dir := fixtureStore(t)
	store.loadErr = errors.New("storage must not be consulted")
	finder := &stubFinder{inst: store.instances[0]}
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, finder).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.png", pngMagic()))
	if rr.Code != http.StatusOK || finder.calls != 1 {
		t.Fatalf("code=%d calls=%d body=%s", rr.Code, finder.calls, rr.Body.String())
	}
	var resp sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(resp.Path) != filepath.Join(dir, "uploads") || resp.Filename != filepath.Base(resp.Path) {
		t.Errorf("unexpected response %+v", resp)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type %q", ct)
	}
}

func TestSessionImageUpload_FinderMiss_FallsBackToStorage(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	finder := &stubFinder{}
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, finder).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.png", pngMagic()))
	if rr.Code != http.StatusOK || finder.calls != 1 {
		t.Fatalf("code=%d calls=%d body=%s", rr.Code, finder.calls, rr.Body.String())
	}
}

func TestSessionImageUpload_LoadInstancesError_500(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	store.loadErr = errors.New("boom")
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.png", pngMagic()))
	assertBody(t, rr, http.StatusInternalServerError, "internal error\n")
}

func TestSessionImageUpload_UploadsPathIsAFile_500(t *testing.T) {
	t.Parallel()
	store, dir := fixtureStore(t)
	if err := os.WriteFile(filepath.Join(dir, "uploads"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.png", pngMagic()))
	assertBody(t, rr, http.StatusInternalServerError, "failed to create uploads directory\n")
}

func TestSessionImageUpload_UploadsDirNotWritable_500(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	store, dir := fixtureStore(t)
	up := filepath.Join(dir, "uploads")
	if err := os.Mkdir(up, 0o500); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.png", pngMagic()))
	assertBody(t, rr, http.StatusInternalServerError, "failed to save file\n")
	if entries, _ := os.ReadDir(up); len(entries) != 0 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestSessionImageUpload_SavedFileKeepsFullContent(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	payload := append(jpegMagic(), []byte("tail-beyond-first-512-bytes")...)
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "p.jpg", payload))
	var resp sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(resp.Path)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("saved content differs (err=%v, %d vs %d bytes)", err, len(got), len(payload))
	}
	if !strings.HasSuffix(resp.Filename, "-p.jpg") {
		t.Errorf("filename %q lost the sanitized original name", resp.Filename)
	}
}

func TestSanitizeFilename_Branches(t *testing.T) {
	t.Parallel()
	longExt := "a." + strings.Repeat("x", 150)
	cases := map[string]string{
		"a\\b.png":                      "a_b.png",
		"nul\x00.png":                   "nul_.png",
		longExt:                         longExt[:100],
		"dir/":                          "dir",
		"/":                             "_",
		strings.Repeat("é", 80) + ".go": (strings.Repeat("é", 80) + ".go")[:97] + ".go",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBacklogAttachmentUpload_WrongMethod_405(t *testing.T) {
	t.Parallel()
	h, err := NewBacklogAttachmentUploadHandler(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, httptest.NewRequest(http.MethodPut, "/x", nil))
	assertBody(t, rr, http.StatusMethodNotAllowed, "method not allowed\n")
}

func TestBacklogAttachmentUpload_BadMultipart_400(t *testing.T) {
	t.Parallel()
	h, _ := NewBacklogAttachmentUploadHandler(t.TempDir())
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, rawMultipartRequest(t, "/x", []byte("nope"), "application/json"))
	assertBody(t, rr, http.StatusBadRequest, "bad multipart form\n")
}

func TestBacklogAttachmentUpload_BodyOverCap_413(t *testing.T) {
	t.Parallel()
	h, _ := NewBacklogAttachmentUploadHandler(t.TempDir())
	big := append(pngMagic(), bytes.Repeat([]byte{0}, backlogAttachmentMaxBytes+128*1024)...)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, buildBacklogAttachmentRequest(t, "big.png", big))
	assertBody(t, rr, http.StatusRequestEntityTooLarge, "file too large (max 10 MB)\n")
}

func TestBacklogAttachmentUpload_UnsupportedTypeMessageNamesSniffedType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	h, _ := NewBacklogAttachmentUploadHandler(dir)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, buildBacklogAttachmentRequest(t, "x.png", []byte("plain text, not an image")))
	assertBody(t, rr, http.StatusUnsupportedMediaType,
		"unsupported file type: text/plain; charset=utf-8 (only PNG, JPEG, GIF, and WebP images are allowed)\n")
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("rejected upload left files: %v", entries)
	}
}

func TestBacklogAttachmentUpload_ExtensionForcedFromSniff_ModeAndName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	h, _ := NewBacklogAttachmentUploadHandler(dir)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, buildBacklogAttachmentRequest(t, "evil.php.png", jpegMagic()))
	var resp backlogAttachmentUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(resp.Filename, "-evil.php.jpg") || filepath.Dir(resp.Path) != dir {
		t.Errorf("unexpected response %+v", resp)
	}
	info, err := os.Stat(resp.Path)
	if err != nil || info.Mode().Perm() != backlogAttachmentFileMode {
		t.Errorf("mode/err = %v / %v", info, err)
	}
}

func TestBacklogAttachmentUpload_DirRemoved_500(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "gone")
	h, err := NewBacklogAttachmentUploadHandler(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, buildBacklogAttachmentRequest(t, "a.png", pngMagic()))
	assertBody(t, rr, http.StatusInternalServerError, "failed to save file\n")
}

func TestNewAttachmentHandlers_DirCannotBeCreated(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(blocker, "sub")
	for name, ctor := range map[string]func(string) (*BacklogAttachmentUploadHandler, error){
		"backlog":  NewBacklogAttachmentUploadHandler,
		"workflow": NewWorkflowAttachmentUploadHandler,
	} {
		h, err := ctor(target)
		if h != nil || err == nil || !strings.HasPrefix(err.Error(), "cannot create attachment dir \""+target+"\": ") {
			t.Errorf("%s: handler=%v err=%v", name, h, err)
		}
	}
}

func TestNewAttachmentHandlers_LogPrefixAndDirMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	b, _ := NewBacklogAttachmentUploadHandler(filepath.Join(root, "b"))
	w, _ := NewWorkflowAttachmentUploadHandler(filepath.Join(root, "w"))
	if b.logPrefix != "BacklogAttachmentUpload" || w.logPrefix != "WorkflowAttachmentUpload" {
		t.Errorf("prefixes %q %q", b.logPrefix, w.logPrefix)
	}
	info, err := os.Stat(filepath.Join(root, "b"))
	if err != nil || info.Mode().Perm()&^backlogAttachmentDirMode != 0 || info.Mode().Perm()&0o007 != 0 {
		t.Errorf("dir mode %v err %v", info, err)
	}
}

func encodedBody(t *testing.T, data []byte, contentType, name string) map[string]string {
	t.Helper()
	return map[string]string{
		"data":             base64.StdEncoding.EncodeToString(data),
		"contentType":      contentType,
		"originalFilename": name,
	}
}

func TestFileUpload_EmptyData_400(t *testing.T) {
	t.Parallel()
	h, _ := newFileUploadHandler(t)
	assertBody(t, postJSON(t, h, map[string]string{"data": ""}), http.StatusBadRequest, "data is required\n")
}

func TestFileUpload_DecodesToNothing_400(t *testing.T) {
	t.Parallel()
	h, _ := newFileUploadHandler(t)
	assertBody(t, postJSON(t, h, map[string]string{"data": "\n"}), http.StatusBadRequest, "file data is empty\n")
}

func TestFileUpload_InvalidJSONAndBase64Messages(t *testing.T) {
	t.Parallel()
	h, _ := newFileUploadHandler(t)
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{")))
	assertBody(t, rr, http.StatusBadRequest, "invalid JSON body\n")
	assertBody(t, postJSON(t, h, map[string]string{"data": "!!!"}), http.StatusBadRequest, "invalid base64 data\n")
}

func TestFileUpload_SizeBoundary(t *testing.T) {
	t.Parallel()
	h, dir := newFileUploadHandler(t)
	rr := postJSON(t, h, encodedBody(t, bytes.Repeat([]byte{1}, maxUploadBytes), "application/octet-stream", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("exactly the cap must be accepted, got %d %s", rr.Code, rr.Body.String())
	}
	rr = postJSON(t, h, encodedBody(t, bytes.Repeat([]byte{1}, maxUploadBytes+1), "application/octet-stream", ""))
	assertBody(t, rr, http.StatusRequestEntityTooLarge, "file exceeds 20 MB limit\n")
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("want exactly the accepted file on disk, got %d", len(entries))
	}
}

func TestFileUpload_BodyBeyondReaderCap_InvalidJSON(t *testing.T) {
	t.Parallel()
	h, _ := newFileUploadHandler(t)
	huge := `{"data":"` + strings.Repeat("A", maxUploadBytes*4/3+8192) + `"}`
	rr := httptest.NewRecorder()
	h.HandleUpload(rr, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(huge)))
	assertBody(t, rr, http.StatusBadRequest, "invalid JSON body\n")
}

func TestFileUpload_ResponseModeAndContent(t *testing.T) {
	t.Parallel()
	h, dir := newFileUploadHandler(t)
	payload := []byte("hello clipboard")
	rr := postJSON(t, h, encodedBody(t, payload, "text/plain; charset=utf-8", "notes.md"))
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v", rr.Code, rr.Header())
	}
	var resp fileUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(resp.Path) != dir || !strings.HasPrefix(filepath.Base(resp.Path), "paste-") || !strings.HasSuffix(resp.Path, ".txt") {
		t.Errorf("path %q", resp.Path)
	}
	info, err := os.Stat(resp.Path)
	if err != nil || info.Mode().Perm() != uploadFileMode {
		t.Fatalf("stat %v %v", info, err)
	}
	if got, _ := os.ReadFile(resp.Path); !bytes.Equal(got, payload) {
		t.Errorf("content %q", got)
	}
}

func TestFileUpload_DirRemoved_500(t *testing.T) {
	t.Parallel()
	h, dir := newFileUploadHandler(t)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	rr := postJSON(t, h, encodedBody(t, []byte("x"), "text/plain", ""))
	assertBody(t, rr, http.StatusInternalServerError, "failed to save file\n")
}

func TestNewFileUploadHandler_UncreatableDirDoesNotPanic(t *testing.T) {
	t.Parallel()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewFileUploadHandler(filepath.Join(blocker, "sub"))
	if h == nil {
		t.Fatal("nil handler")
	}
	rr := postJSON(t, h, encodedBody(t, []byte("x"), "text/plain", ""))
	assertBody(t, rr, http.StatusInternalServerError, "failed to save file\n")
}

func TestNewFileUploadHandler_CreatesDirOwnerOnly(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "paste")
	NewFileUploadHandler(dir)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Errorf("dir %v err %v", info, err)
	}
}

func TestCleanOldPasteFiles_KeepsFreshAndDirs_RemovesStale(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stale := filepath.Join(dir, "stale")
	fresh := filepath.Join(dir, "fresh")
	edge := filepath.Join(dir, "edge")
	staleDir := filepath.Join(dir, "olddir")
	for _, p := range []string{stale, fresh, edge} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(staleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-maxPasteFileAge - time.Minute)
	nearEdge := time.Now().Add(-maxPasteFileAge + time.Minute)
	for p, ts := range map[string]time.Time{stale: old, staleDir: old, edge: nearEdge} {
		if err := os.Chtimes(p, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	cleanOldPasteFiles(dir)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file survived: %v", err)
	}
	for _, p := range []string{fresh, edge, staleDir} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s removed: %v", p, err)
		}
	}
	cleanOldPasteFiles(filepath.Join(dir, "missing")) // ReadDir error path must not panic
}
