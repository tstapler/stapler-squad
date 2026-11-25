package uploads

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
)

// The golden transcript was recorded from the pre-move handlers in
// server/services (origin/main af7474d28) over the scenarios below; the leaf must
// reproduce it byte for byte. Set UPLOADS_GOLDEN_RECORD=<path> to re-record.

var (
	goldenTS   = regexp.MustCompile(`\b\d{13}-\d+-`)
	goldenPast = regexp.MustCompile(`paste-\d+`)
)

type goldenLog struct {
	b     strings.Builder
	dir   string
	extra []string
}

func (g *goldenLog) norm(s string) string {
	if g.dir != "" {
		s = strings.ReplaceAll(s, g.dir, "<DIR>")
	}
	for _, e := range g.extra {
		s = strings.ReplaceAll(s, e, "<EXTRA>")
	}
	s = goldenTS.ReplaceAllString(s, "<TS>-<RND>-")
	return goldenPast.ReplaceAllString(s, "paste-<RND>")
}

func (g *goldenLog) section(name string) { fmt.Fprintf(&g.b, "== %s\n", name) }

func (g *goldenLog) response(rr *httptest.ResponseRecorder) {
	hdr := make([]string, 0, len(rr.Header()))
	for k, v := range rr.Header() {
		hdr = append(hdr, k+"="+strings.Join(v, "|"))
	}
	sort.Strings(hdr)
	fmt.Fprintf(&g.b, "  status=%d headers=[%s]\n  body=%q\n", rr.Code, strings.Join(hdr, ";"), g.norm(rr.Body.String()))
}

func (g *goldenLog) tree(root string) {
	var lines []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || p == root {
			return nil
		}
		rel := g.norm(strings.TrimPrefix(p, root))
		if info.IsDir() {
			lines = append(lines, fmt.Sprintf("  dir  %s %o", rel, info.Mode().Perm()))
			return nil
		}
		data, _ := os.ReadFile(p)
		sum := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("  file %s %o %d %s", rel, info.Mode().Perm(), info.Size(), hex.EncodeToString(sum[:6])))
		return nil
	})
	sort.Strings(lines)
	fmt.Fprintf(&g.b, "  tree(%d)\n%s\n", len(lines), strings.Join(lines, "\n"))
}

func gMultipart(fields map[string]string, filename string, body []byte, hasFile bool, target string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = mw.WriteField(k, fields[k])
	}
	if hasFile {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
		h.Set("Content-Type", "application/octet-stream")
		part, _ := mw.CreatePart(h)
		_, _ = part.Write(body)
	}
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, target, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func gPNG(extra int) []byte {
	return append([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 504+extra)...)
}
func gJPEG() []byte { return append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 508)...) }
func gGIF() []byte  { return append([]byte("GIF89a"), make([]byte, 506)...) }
func gWEBP() []byte {
	b := make([]byte, 512)
	copy(b[0:4], "RIFF")
	copy(b[8:14], "WEBPVP")
	return b
}

var goldenNames = []string{
	"photo.jpg", "../../etc/passwd", "/abs/path/x.png", `a\b.png`, "nul\x00.png", "", ".", "..", "dir/",
	strings.Repeat("a", 200) + ".jpg", "a." + strings.Repeat("x", 150), strings.Repeat("é", 80) + ".go",
	"evil.php.png", ".hidden", "no-ext", "UPPER.PNG", "sp ace.png", "x\ny.png",
}

func TestUploadsGolden(t *testing.T) {
	g := &goldenLog{}

	// ---- session handler ----
	run := func(name string, store *fakeInstanceStore, finder instanceFinder, root string, mk func() *http.Request) {
		g.dir = root
		g.section(name)
		rr := httptest.NewRecorder()
		NewSessionImageUploadHandler(store, finder).HandleUpload(rr, mk())
		g.response(rr)
		g.tree(root)
	}
	newStore := func(path string) (*fakeInstanceStore, string) {
		inst := &instance.Instance{}
		inst.UUID = "uuid-1"
		inst.Title = "title-1"
		inst.Path = path
		return &fakeInstanceStore{instances: []*instance.Instance{inst}}, path
	}
	base := func(root string) map[string]string { return map[string]string{"session_id": "uuid-1"} }

	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		root := t.TempDir()
		st, _ := newStore(root)
		run("session/method/"+m, st, nil, root, func() *http.Request {
			return httptest.NewRequest(m, "/api/v1/upload-image", nil)
		})
	}
	{
		root := t.TempDir()
		st, _ := newStore(root)
		run("session/not-multipart", st, nil, root, func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hello"))
			r.Header.Set("Content-Type", "text/plain")
			return r
		})
		run("session/no-content-type", st, nil, root, func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hello"))
		})
		run("session/missing-session-id", st, nil, root, func() *http.Request {
			return gMultipart(nil, "a.png", gPNG(0), true, "/x")
		})
		run("session/missing-file", st, nil, root, func() *http.Request {
			return gMultipart(base(root), "", nil, false, "/x")
		})
		run("session/empty-file", st, nil, root, func() *http.Request {
			return gMultipart(base(root), "a.png", nil, true, "/x")
		})
		run("session/unknown-session", st, nil, root, func() *http.Request {
			return gMultipart(map[string]string{"session_id": "nope"}, "a.png", gPNG(0), true, "/x")
		})
		run("session/by-title", st, nil, root, func() *http.Request {
			return gMultipart(map[string]string{"session_id": "title-1"}, "a.png", gPNG(0), true, "/x")
		})
	}
	{
		root := t.TempDir()
		st, _ := newStore("")
		run("session/no-path", st, nil, root, func() *http.Request {
			return gMultipart(base(root), "a.png", gPNG(0), true, "/x")
		})
		st2, _ := newStore(filepath.Join(root, "missing"))
		run("session/path-missing-on-disk", st2, nil, root, func() *http.Request {
			return gMultipart(base(root), "a.png", gPNG(0), true, "/x")
		})
	}
	{
		root := t.TempDir()
		_ = os.WriteFile(filepath.Join(root, "uploads"), []byte("x"), 0o600)
		st, _ := newStore(root)
		run("session/uploads-is-file", st, nil, root, func() *http.Request {
			return gMultipart(base(root), "a.png", gPNG(0), true, "/x")
		})
	}
	{
		root := t.TempDir()
		st, _ := newStore(root)
		st.loadErr = fmt.Errorf("load failed")
		run("session/load-error", st, nil, root, func() *http.Request {
			return gMultipart(base(root), "a.png", gPNG(0), true, "/x")
		})
	}
	for i, name := range goldenNames {
		root := t.TempDir()
		st, _ := newStore(root)
		nm := name
		run(fmt.Sprintf("session/filename/%d", i), st, nil, root, func() *http.Request {
			return gMultipart(base(root), nm, gJPEG(), true, "/x")
		})
	}
	for _, c := range []struct {
		n    string
		body []byte
	}{
		{"jpeg", gJPEG()}, {"png", gPNG(0)}, {"gif", gGIF()}, {"webp", gWEBP()},
		{"text", []byte("plain text file")}, {"one-byte", []byte{1}}, {"512", make([]byte, 512)}, {"513", make([]byte, 513)},
		{"exact-10MB", bytes.Repeat([]byte{7}, 10*1024*1024)},
		{"10MB+1", bytes.Repeat([]byte{7}, 10*1024*1024+1)},
		{"10MB+100k", bytes.Repeat([]byte{7}, 10*1024*1024+100*1024)},
	} {
		root := t.TempDir()
		st, _ := newStore(root)
		body := c.body
		run("session/content/"+c.n, st, nil, root, func() *http.Request {
			return gMultipart(base(root), "f.bin", body, true, "/x")
		})
	}
	{
		root := t.TempDir()
		st, _ := newStore(root)
		other := &instance.Instance{}
		other.UUID, other.Path = "uuid-1", t.TempDir()
		g.extra = []string{other.Path}
		run("session/finder-hit", st, &goldenFinder{inst: other}, root, func() *http.Request {
			return gMultipart(base(root), "a.png", gPNG(0), true, "/x")
		})
		g.dir = other.Path
		g.tree(other.Path)
		run("session/finder-miss", st, &goldenFinder{}, root, func() *http.Request {
			return gMultipart(base(root), "a.png", gPNG(0), true, "/x")
		})
		for i := 0; i < 3; i++ {
			run(fmt.Sprintf("session/repeat/%d", i), st, nil, root, func() *http.Request {
				return gMultipart(base(root), "same.png", gPNG(i), true, "/x")
			})
		}
	}

	// ---- backlog / workflow attachment handler ----
	attach := func(name string, wf bool, mk func(dir string) *http.Request, prep func(dir string)) {
		root := t.TempDir()
		dir := filepath.Join(root, "att")
		g.dir = root
		g.section(name)
		var h *BacklogAttachmentUploadHandler
		var err error
		if wf {
			h, err = NewWorkflowAttachmentUploadHandler(dir)
		} else {
			h, err = NewBacklogAttachmentUploadHandler(dir)
		}
		if err != nil {
			fmt.Fprintf(&g.b, "  ctor-error=%q\n", g.norm(err.Error()))
			return
		}
		if prep != nil {
			prep(dir)
		}
		rr := httptest.NewRecorder()
		if wf {
			h.HandleWorkflowUpload(rr, mk(dir))
		} else {
			h.HandleUpload(rr, mk(dir))
		}
		g.response(rr)
		g.tree(root)
	}
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		attach("backlog/method/"+m, false, func(string) *http.Request { return httptest.NewRequest(m, "/x", nil) }, nil)
	}
	attach("backlog/not-multipart", false, func(string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("hi"))
		r.Header.Set("Content-Type", "application/json")
		return r
	}, nil)
	attach("backlog/missing-file", false, func(string) *http.Request { return gMultipart(nil, "", nil, false, "/x") }, nil)
	attach("backlog/empty-file", false, func(string) *http.Request { return gMultipart(nil, "a.png", nil, true, "/x") }, nil)
	attach("backlog/dir-removed", false, func(string) *http.Request { return gMultipart(nil, "a.png", gPNG(0), true, "/x") },
		func(dir string) { _ = os.Remove(dir) })
	for _, c := range []struct {
		n    string
		body []byte
	}{
		{"png", gPNG(0)}, {"jpeg", gJPEG()}, {"gif", gGIF()}, {"webp", gWEBP()},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"html", []byte("<!DOCTYPE html><html><script>1</script></html>")},
		{"text", []byte("plain")}, {"pdf", []byte("%PDF-1.4 hello")}, {"bmp", append([]byte("BM"), make([]byte, 60)...)},
		{"php-in-png-name", []byte("<?php system($_GET[0]); ?>")},
		{"png-header-then-html", append(gPNG(0), []byte("<script>1</script>")...)},
		{"exact-10MB", append(gPNG(0), bytes.Repeat([]byte{0}, 10*1024*1024-512)...)},
		{"10MB+1", append(gPNG(0), bytes.Repeat([]byte{0}, 10*1024*1024-511)...)},
		{"10MB+100k", append(gPNG(0), bytes.Repeat([]byte{0}, 10*1024*1024+100*1024)...)},
	} {
		body := c.body
		attach("backlog/content/"+c.n, false, func(string) *http.Request { return gMultipart(nil, "evil.png", body, true, "/x") }, nil)
	}
	for i, name := range goldenNames {
		nm := name
		attach(fmt.Sprintf("backlog/filename/%d", i), false, func(string) *http.Request { return gMultipart(nil, nm, gPNG(0), true, "/x") }, nil)
	}
	attach("workflow/png", true, func(string) *http.Request { return gMultipart(nil, "plan.png", gPNG(0), true, "/x") }, nil)
	attach("workflow/svg", true, func(string) *http.Request {
		return gMultipart(nil, "plan.png", []byte("<svg><script/></svg>"), true, "/x")
	}, nil)
	attach("workflow/get", true, func(string) *http.Request { return httptest.NewRequest(http.MethodGet, "/x", nil) }, nil)
	{
		root := t.TempDir()
		blocker := filepath.Join(root, "f")
		_ = os.WriteFile(blocker, nil, 0o600)
		g.dir = root
		g.section("ctor/uncreatable")
		_, err := NewBacklogAttachmentUploadHandler(filepath.Join(blocker, "sub"))
		fmt.Fprintf(&g.b, "  err=%q\n", g.norm(fmt.Sprint(err)))
		_, err = NewWorkflowAttachmentUploadHandler(filepath.Join(blocker, "sub"))
		fmt.Fprintf(&g.b, "  err=%q\n", g.norm(fmt.Sprint(err)))
	}

	// ---- clipboard file handler ----
	fileRun := func(name string, raw func() io.Reader, method string, prep func(dir string)) {
		root := t.TempDir()
		dir := filepath.Join(root, "paste")
		g.dir = root
		g.section(name)
		if prep != nil {
			_ = os.MkdirAll(dir, 0o700)
			prep(dir)
		}
		h := NewFileUploadHandler(dir)
		rr := httptest.NewRecorder()
		h.HandleUpload(rr, httptest.NewRequest(method, "/api/upload/file", raw()))
		g.response(rr)
		g.tree(root)
	}
	enc := func(data []byte, ct, name string) func() io.Reader {
		return func() io.Reader {
			b, _ := json.Marshal(map[string]string{"data": base64.StdEncoding.EncodeToString(data), "contentType": ct, "originalFilename": name})
			return bytes.NewReader(b)
		}
	}
	str := func(s string) func() io.Reader { return func() io.Reader { return strings.NewReader(s) } }
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		fileRun("file/method/"+m, str("{}"), m, nil)
	}
	fileRun("file/invalid-json", str("{"), http.MethodPost, nil)
	fileRun("file/empty-body", str(""), http.MethodPost, nil)
	fileRun("file/json-array", str("[]"), http.MethodPost, nil)
	fileRun("file/no-data", str(`{"contentType":"text/plain"}`), http.MethodPost, nil)
	fileRun("file/empty-data", str(`{"data":""}`), http.MethodPost, nil)
	fileRun("file/bad-base64", str(`{"data":"!!!"}`), http.MethodPost, nil)
	fileRun("file/newline-data", str(`{"data":"\n"}`), http.MethodPost, nil)
	fileRun("file/unpadded-base64", str(`{"data":"aGVsbG8"}`), http.MethodPost, nil)
	fileRun("file/trailing-json", str(`{"data":"aGVsbG8="} trailing`), http.MethodPost, nil)
	fileRun("file/size/exact-20MB", enc(bytes.Repeat([]byte{1}, 20*1024*1024), "", ""), http.MethodPost, nil)
	fileRun("file/size/20MB+1", enc(bytes.Repeat([]byte{1}, 20*1024*1024+1), "", ""), http.MethodPost, nil)
	fileRun("file/size/body-over-reader-cap", func() io.Reader {
		return strings.NewReader(`{"data":"` + strings.Repeat("A", 20*1024*1024*4/3+8192) + `"}`)
	}, http.MethodPost, nil)
	mimes := []string{
		"", "image/png", "IMAGE/PNG", "image/png; charset=binary", " image/png ", "image/jpeg", "image/jpg", "image/gif", "image/webp",
		"image/svg+xml", "image/bmp", "image/tiff", "image/ico", "image/x-icon", "text/plain", "text/html", "text/css", "text/javascript",
		"text/typescript", "text/x-python", "text/x-go", "text/x-rust", "text/x-c", "text/x-c++", "text/x-java", "text/x-ruby", "text/x-sh",
		"text/xml", "text/csv", "text/markdown", "application/json", "application/javascript", "application/typescript", "application/xml",
		"application/yaml", "application/x-yaml", "application/toml", "application/pdf", "application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/zip", "application/gzip", "application/x-gzip",
		"application/x-tar", "application/x-bzip2", "application/x-7z-compressed", "application/x-rar-compressed",
		"application/octet-stream", "application/unknown", "weird", "image/", "/",
	}
	for i, m := range mimes {
		fileRun(fmt.Sprintf("file/mime/%d", i), enc([]byte("payload"), m, ""), http.MethodPost, nil)
	}
	names := []string{
		"a.PNG", "a.tar.gz", "a.", "a", ".bashrc", "x.p/hp", "../x.sh", "x.verylongextension1234", "x.sh;rm", "x.a b", "x.é", "dir/a.go",
		"x." + strings.Repeat("z", 40), "x.tar", "x.GO", `x.\a`, "x.a\x00b",
	}
	for i, n := range names {
		fileRun(fmt.Sprintf("file/origname/%d/no-mime", i), enc([]byte("payload"), "", n), http.MethodPost, nil)
		fileRun(fmt.Sprintf("file/origname/%d/unknown-mime", i), enc([]byte("payload"), "application/unknown", n), http.MethodPost, nil)
		fileRun(fmt.Sprintf("file/origname/%d/known-mime", i), enc([]byte("payload"), "image/png", n), http.MethodPost, nil)
	}
	fileRun("file/plain-text", enc([]byte("x"), "text/plain", ""), http.MethodPost, nil)
	{
		root := t.TempDir()
		blocker := filepath.Join(root, "f")
		_ = os.WriteFile(blocker, nil, 0o600)
		g.dir = root
		g.section("file/uncreatable-dir")
		h := NewFileUploadHandler(filepath.Join(blocker, "sub"))
		rr := httptest.NewRecorder()
		h.HandleUpload(rr, httptest.NewRequest(http.MethodPost, "/x", enc([]byte("x"), "text/plain", "")()))
		g.response(rr)
	}
	fileRun("file/startup-eviction", enc([]byte("x"), "text/plain", ""), http.MethodPost, func(dir string) {
		old := time.Now().Add(-25 * time.Hour)
		fresh := time.Now().Add(-23 * time.Hour)
		for name, ts := range map[string]time.Time{"stale-a": old, "stale-b": old, "fresh-a": fresh, "fresh-b": time.Now()} {
			p := filepath.Join(dir, name)
			_ = os.WriteFile(p, []byte(name), 0o600)
			_ = os.Chtimes(p, ts, ts)
		}
		_ = os.Mkdir(filepath.Join(dir, "olddir"), 0o700)
		_ = os.Chtimes(filepath.Join(dir, "olddir"), old, old)
	})

	got := g.b.String()
	if rec := os.Getenv("UPLOADS_GOLDEN_RECORD"); rec != "" {
		if err := os.WriteFile(rec, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile("testdata/uploads.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("golden mismatch at line %d:\n got: %s\nwant: %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("golden length mismatch: got %d lines, want %d", len(gl), len(wl))
	}
}

type goldenFinder struct{ inst *instance.Instance }

func (f *goldenFinder) FindInstance(string) *instance.Instance { return f.inst }
