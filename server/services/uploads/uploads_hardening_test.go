package uploads

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectionBodiesAreSingleMessages(t *testing.T) {
	t.Parallel()
	store, _ := fixtureStore(t)
	s := NewSessionImageUploadHandler(store, nil)
	b, _ := NewBacklogAttachmentUploadHandler(t.TempDir())
	f, _ := newFileUploadHandler(t)

	rr := httptest.NewRecorder()
	s.HandleUpload(rr, buildUploadRequest(t, "session-123", "e.jpg", []byte{}))
	assertBody(t, rr, http.StatusBadRequest, "uploaded file is empty\n")

	rr = httptest.NewRecorder()
	s.HandleUpload(rr, buildUploadRequest(t, "", "a.jpg", jpegMagic()))
	assertBody(t, rr, http.StatusBadRequest, "session_id is required\n")

	rr = httptest.NewRecorder()
	s.HandleUpload(rr, buildUploadRequest(t, "nope", "a.jpg", jpegMagic()))
	assertBody(t, rr, http.StatusNotFound, "session not found\n")

	rr = httptest.NewRecorder()
	b.HandleUpload(rr, buildBacklogAttachmentRequest(t, "e.png", []byte{}))
	assertBody(t, rr, http.StatusBadRequest, "uploaded file is empty\n")

	rr = httptest.NewRecorder()
	f.HandleUpload(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	assertBody(t, rr, http.StatusMethodNotAllowed, "method not allowed\n")

	rr = httptest.NewRecorder()
	s.HandleUpload(rr, buildUploadRequestNoFile(t, "session-123"))
	if rr.Code != http.StatusBadRequest || !strings.HasPrefix(rr.Body.String(), "file field required: ") || strings.Count(rr.Body.String(), "\n") != 1 {
		t.Errorf("session missing file: %d %q", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	b.HandleUpload(rr, buildBacklogAttachmentRequest(t, "", nil))
	if rr.Code != http.StatusBadRequest || !strings.HasPrefix(rr.Body.String(), "file field required: ") || strings.Count(rr.Body.String(), "\n") != 1 {
		t.Errorf("backlog missing file: %d %q", rr.Code, rr.Body.String())
	}
}

func TestSessionImageUpload_NoPathAndMissingDir_SingleMessage(t *testing.T) {
	t.Parallel()
	store, dir := fixtureStore(t)
	store.instances[0].Path = ""
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.jpg", jpegMagic()))
	assertBody(t, rr, http.StatusUnprocessableEntity, "session has no working directory\n")

	store.instances[0].Path = filepath.Join(dir, "gone")
	rr = httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.jpg", jpegMagic()))
	assertBody(t, rr, http.StatusUnprocessableEntity, "session working directory is not accessible\n")
}

func TestModesAreOwnerOnly(t *testing.T) {
	t.Parallel()
	store, dir := fixtureStore(t)
	rr := httptest.NewRecorder()
	NewSessionImageUploadHandler(store, nil).HandleUpload(rr, buildUploadRequest(t, "session-123", "a.jpg", jpegMagic()))
	var sr sessionImageUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&sr); err != nil {
		t.Fatal(err)
	}
	assertModes(t, map[string]os.FileMode{filepath.Join(dir, "uploads"): 0o750, sr.Path: 0o600})

	bdir := filepath.Join(t.TempDir(), "att")
	b, _ := NewBacklogAttachmentUploadHandler(bdir)
	rr = httptest.NewRecorder()
	b.HandleUpload(rr, buildBacklogAttachmentRequest(t, "a.png", pngMagic()))
	var br backlogAttachmentUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&br); err != nil {
		t.Fatal(err)
	}
	assertModes(t, map[string]os.FileMode{bdir: 0o750, br.Path: 0o600})

	pdir := filepath.Join(t.TempDir(), "paste")
	f := NewFileUploadHandler(pdir)
	rr = postJSON(t, f, encodedBody(t, []byte("x"), "text/plain", ""))
	var fr fileUploadResponse
	if err := json.NewDecoder(rr.Body).Decode(&fr); err != nil {
		t.Fatal(err)
	}
	assertModes(t, map[string]os.FileMode{pdir: 0o700, fr.Path: 0o600})
}

func assertModes(t *testing.T, want map[string]os.FileMode) {
	t.Helper()
	for path, mode := range want {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s: mode %v err %v, want %o", path, info, err, mode)
		}
	}
}

func TestSanitizeExtension_CharacterClass(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"a.0":          ".0",
		"a.9":          ".9",
		"a.z":          ".z",
		"a.Z":          ".z",
		"a.A":          ".a",
		"a./":          "",
		"a.:":          "",
		"a.@":          "",
		"a.[":          "",
		"a.`":          "",
		"a.{":          "",
		"a.mp4":        ".mp4",
		"a.a-b_c":      ".abc",
		"a.012345678":  ".012345678",
		"a.0123456789": "",
		"a..":          "",
		"a.":           "",
		"a":            "",
		"a.ünï":        ".n",
	}
	for in, want := range cases {
		if got := sanitizeExtension(in); got != want {
			t.Errorf("sanitizeExtension(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFilename_TruncationBoundaries(t *testing.T) {
	t.Parallel()
	exact := strings.Repeat("a", 96) + ".png"
	extIs100 := "a." + strings.Repeat("x", 99)
	cases := map[string]string{
		exact:                            exact,
		strings.Repeat("a", 97) + ".png": strings.Repeat("a", 96) + ".png",
		extIs100:                         extIs100[:100],
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want || len(got) > 100 {
			t.Errorf("sanitizeFilename(len %d) = %q (len %d), want %q", len(in), got, len(got), want)
		}
	}
}
