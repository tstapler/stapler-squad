package middleware

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A built web-app/out must not contain a file the login page needs that the auth
// middleware would redirect to /login (a lockout). Every file is checked, not just
// index.html. The Bun build's verify-output runs the same walk in TypeScript; this
// is the independent Go-side check against the real isExempt and StaticFileServer.

const (
	builtOutRelPath   = "../../web-app/out"
	allowlistRelPath  = "testdata/public-allowlist.json"
	immutableCacheCtl = "public, max-age=31536000, immutable"
)

// spaShell is served as the fallback for every deep link, never requested by its own path.
var spaShell = allowlistEntry{Path: "index.html", Reason: "SPA shell; StaticFileServer serves it as the fallback for every deep link"}

type allowlistEntry struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type outDirOutcome int

const (
	outDirReady outDirOutcome = iota
	outDirSkip
	outDirFail
)

// classifyOutDir decides what a missing or empty out/ means: locally the test skips,
// in CI (require) it fails so a mis-wired build step cannot pass silently.
func classifyOutDir(dir string, inCI bool) (outDirOutcome, string) {
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) > 0 {
		return outDirReady, ""
	}
	msg := dir + " is missing or empty; run `make web-build` first"
	if inCI {
		return outDirFail, msg
	}
	return outDirSkip, msg
}

func builtOutDir(t *testing.T, inCI bool) string {
	t.Helper()
	dir, err := filepath.Abs(builtOutRelPath)
	require.NoError(t, err)
	switch outcome, msg := classifyOutDir(dir, inCI); outcome {
	case outDirFail:
		t.Fatal(msg)
	case outDirSkip:
		t.Skip(msg)
	case outDirReady:
	}
	return dir
}

func requireBuiltOut() bool {
	return os.Getenv("CI") != "" || os.Getenv("REQUIRE_BUILT_OUT") == "1"
}

func loadAllowlist(t *testing.T) []allowlistEntry {
	t.Helper()
	data, err := os.ReadFile(allowlistRelPath)
	require.NoError(t, err)
	var entries []allowlistEntry
	require.NoError(t, json.Unmarshal(data, &entries))
	return entries
}

func validateAllowlist(entries []allowlistEntry) []string {
	var problems []string
	for i, e := range entries {
		if e.Path == "" {
			problems = append(problems, fmt.Sprintf("allowlist entry %d: empty path", i))
		}
		if strings.TrimSpace(e.Reason) == "" {
			problems = append(problems, "allowlist entry "+e.Path+": empty reason")
		}
	}
	return problems
}

func allowlistGlob(p string) *regexp.Regexp {
	return regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(p), `\*`, "[^/]*") + "$")
}

func isAllowlisted(rel string, allowlist []allowlistEntry) bool {
	for _, e := range append([]allowlistEntry{spaShell}, allowlist...) {
		if allowlistGlob(e.Path).MatchString(rel) {
			return true
		}
	}
	return false
}

func isExemptPath(urlPath string) bool {
	return isExempt(httptest.NewRequest(http.MethodGet, urlPath, nil))
}

func listFiles(fsys fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	sort.Strings(files)
	return files, err
}

var (
	htmlRef     = regexp.MustCompile(`(?:src|href)=["']([^"']+)["']`)
	cssURLRef   = regexp.MustCompile(`url\(\s*["']?([^"')\s]+)["']?\s*\)`)
	jsImportRef = regexp.MustCompile(`import\(\s*["']([^"']+)["']\s*\)`)
	fileLike    = regexp.MustCompile(`\.(?:js|mjs|css|woff2?|png|jpe?g|svg|ico|webp|json|wasm|map)$`)
)

// referencedFiles returns the out-relative paths that file refers to, plus problems
// for references that escape out/. Only file-like references are returned: hrefs to
// app routes (/, /login) are served by the SPA fallback and are not files.
func referencedFiles(file string, content []byte) (refs, problems []string) {
	var raw []string
	switch path.Ext(file) {
	case ".html":
		for _, m := range htmlRef.FindAllSubmatch(content, -1) {
			raw = append(raw, string(m[1]))
		}
	case ".css":
		for _, m := range cssURLRef.FindAllSubmatch(content, -1) {
			raw = append(raw, string(m[1]))
		}
	case ".js", ".mjs":
		for _, m := range jsImportRef.FindAllSubmatch(content, -1) {
			raw = append(raw, string(m[1]))
		}
	}
	for _, ref := range raw {
		ref = strings.SplitN(strings.SplitN(ref, "#", 2)[0], "?", 2)[0]
		if ref == "" || strings.HasPrefix(ref, "data:") || strings.Contains(ref, "://") || !fileLike.MatchString(ref) {
			continue
		}
		resolved := strings.TrimPrefix(ref, "/")
		if !strings.HasPrefix(ref, "/") {
			resolved = path.Join(path.Dir(file), ref)
		}
		resolved = path.Clean(resolved)
		if strings.HasPrefix(resolved, "..") {
			problems = append(problems, file+" references "+ref+", which is outside out/")
			continue
		}
		refs = append(refs, resolved)
	}
	return refs, problems
}

// checkBuiltAssets returns one problem per violation; an empty result means the tree is safe to embed.
func checkBuiltAssets(fsys fs.FS, allowlist []allowlistEntry) []string {
	problems := validateAllowlist(allowlist)
	files, err := listFiles(fsys)
	if err != nil {
		return append(problems, "walk: "+err.Error())
	}
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
	}

	problems = append(problems, checkExemptionAndCaching(fsys, files, allowlist)...)
	return append(problems, checkReferences(fsys, files, present)...)
}

func checkExemptionAndCaching(fsys fs.FS, files []string, allowlist []allowlistEntry) []string {
	var problems []string
	static := StaticFileServer(fsys, "index.html")
	for _, f := range files {
		if !isExemptPath("/"+f) && !isAllowlisted(f, allowlist) {
			problems = append(problems, f+" is neither under an auth-exempt prefix nor on the public allowlist (it would redirect to /login)")
		}
		if strings.HasPrefix(f, "_next/") {
			w := httptest.NewRecorder()
			static.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+f, nil))
			if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != immutableCacheCtl {
				problems = append(problems, f+" is not served 200 with the immutable cache header")
			}
		}
	}
	return problems
}

func checkReferences(fsys fs.FS, files []string, present map[string]bool) []string {
	var problems []string
	for _, f := range files {
		if ext := path.Ext(f); ext != ".html" && ext != ".css" && ext != ".js" && ext != ".mjs" {
			continue
		}
		content, readErr := fs.ReadFile(fsys, f)
		if readErr != nil {
			problems = append(problems, "read "+f+": "+readErr.Error())
			continue
		}
		refs, refProblems := referencedFiles(f, content)
		problems = append(problems, refProblems...)
		for _, ref := range refs {
			if !present[ref] {
				problems = append(problems, f+" references "+ref+", which does not exist in out/")
			}
		}
	}
	return problems
}

func TestBuiltAssets_should_BindEveryBuiltFileToTheAuthExemption(t *testing.T) {
	out := builtOutDir(t, requireBuiltOut())
	problems := checkBuiltAssets(os.DirFS(out), loadAllowlist(t))
	assert.Empty(t, problems, "built files the login page could not load unauthenticated")
}

func TestBuiltOutDir_should_SkipLocallyAndFailInCI_When_OutIsMissingOrEmpty(t *testing.T) {
	t.Parallel()
	empty := t.TempDir()
	missing := filepath.Join(empty, "nope")

	for _, dir := range []string{empty, missing} {
		outcome, msg := classifyOutDir(dir, false)
		assert.Equal(t, outDirSkip, outcome)
		assert.Contains(t, msg, "make web-build")
		outcome, _ = classifyOutDir(dir, true)
		assert.Equal(t, outDirFail, outcome)
	}

	require.NoError(t, os.WriteFile(filepath.Join(empty, "index.html"), []byte("x"), 0o600))
	for _, req := range []bool{false, true} {
		outcome, _ := classifyOutDir(empty, req)
		assert.Equal(t, outDirReady, outcome)
	}
}

func goodTree() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                      {Data: []byte(`<script src="/_next/static/chunks/entry-1.js"></script><link href="/_next/static/chunks/entry-1.css">`)},
		"_next/static/chunks/entry-1.js":  {Data: []byte(`import("./lazy-2.js")`)},
		"_next/static/chunks/lazy-2.js":   {Data: []byte(`1`)},
		"_next/static/chunks/entry-1.css": {Data: []byte(`@font-face{src:url(/_next/static/media/f.woff2)}`)},
		"_next/static/media/f.woff2":      {Data: []byte(`font`)},
		"push-sw.js":                      {Data: []byte(`1`)},
	}
}

var pushSW = allowlistEntry{Path: "push-sw.js", Reason: "registered after login"}

func TestCheckBuiltAssets_should_PassAGoodTree(t *testing.T) {
	t.Parallel()
	assert.Empty(t, checkBuiltAssets(goodTree(), []allowlistEntry{pushSW}))
}

func TestCheckBuiltAssets_should_NameFilesOutsideTheExemptionAndAllowlist(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"assets/a.js", "entry.js", "media/f.woff2"} {
		tree := goodTree()
		tree[bad] = &fstest.MapFile{Data: []byte("1")}
		problems := checkBuiltAssets(tree, []allowlistEntry{pushSW})
		require.Len(t, problems, 1, bad)
		assert.Contains(t, problems[0], bad)
	}
}

func TestCheckBuiltAssets_should_FailOnAllowlistEntryWithoutReason(t *testing.T) {
	t.Parallel()
	tree := goodTree()
	tree["assets/a.js"] = &fstest.MapFile{Data: []byte("1")}

	reasoned := []allowlistEntry{pushSW, {Path: "assets/a.js", Reason: "fetched after login"}}
	assert.Empty(t, checkBuiltAssets(tree, reasoned))

	unreasoned := []allowlistEntry{pushSW, {Path: "assets/a.js", Reason: " "}}
	problems := checkBuiltAssets(tree, unreasoned)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "assets/a.js")
	assert.Contains(t, problems[0], "empty reason")
}

func TestCheckBuiltAssets_should_FailOnDanglingReferences(t *testing.T) {
	t.Parallel()
	tree := goodTree()
	tree["_next/static/chunks/lazy-2.js"] = &fstest.MapFile{Data: []byte(`import("./missing-3.js")`)}
	tree["_next/static/chunks/entry-1.css"] = &fstest.MapFile{Data: []byte(`a{background:url(../../../../outside.png)}`)}

	problems := strings.Join(checkBuiltAssets(tree, []allowlistEntry{pushSW}), "\n")
	assert.Contains(t, problems, "references _next/static/chunks/missing-3.js, which does not exist")
	assert.Contains(t, problems, "outside.png, which is outside out/")
}

func TestAllowlistGlob_should_MatchWithinOneSegment(t *testing.T) {
	t.Parallel()
	list := []allowlistEntry{{Path: "icons/*", Reason: "r"}}
	assert.True(t, isAllowlisted("icons/icon-72.png", list))
	assert.False(t, isAllowlisted("icons/nested/icon.png", list))
	assert.True(t, isAllowlisted("index.html", nil))
}
