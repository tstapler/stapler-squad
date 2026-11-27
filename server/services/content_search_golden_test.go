package services

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/fsops"
	"github.com/tstapler/stapler-squad/session/domain"
)

// These tests drive content search and the range limiter through NewFileService and the
// Connect wire only, so the same file records the golden before and after a move.
var updateContentSearchGolden = flag.Bool("update-content-search-golden", false, "rewrite testdata/content_search_wiring.golden")

type goldenWorkspaces struct {
	byID    map[string]domain.Workspace
	mu      sync.Mutex
	entered int
	release chan struct{}
}

func (g *goldenWorkspaces) GetWorkspace(id string) (domain.Workspace, error) {
	if id == "broken" {
		return domain.Workspace{}, errors.New("storage offline")
	}
	ws, ok := g.byID[strings.TrimSuffix(id, "-hold")]
	if !ok {
		return domain.Workspace{}, connect.NewError(connect.CodeNotFound, errors.New("no such session"))
	}
	if strings.HasSuffix(id, "-hold") {
		g.mu.Lock()
		g.entered++
		release := g.release
		g.mu.Unlock()
		<-release
	}
	return ws, nil
}

func (g *goldenWorkspaces) arm() func() {
	g.mu.Lock()
	defer g.mu.Unlock()
	ch := make(chan struct{})
	g.release, g.entered = ch, 0
	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

func (g *goldenWorkspaces) waitEntered(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.entered >= n
	}, 10*time.Second, 5*time.Millisecond)
}

type goldenTelemetry struct {
	mu    sync.Mutex
	lines []string
}

func (g *goldenTelemetry) add(format string, a ...any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lines = append(g.lines, fmt.Sprintf(format, a...))
}

func (g *goldenTelemetry) drain() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := g.lines
	g.lines = nil
	return out
}

func (g *goldenTelemetry) RecordWrite(context.Context, string, fsops.WriteOutcomeEvent) {}
func (g *goldenTelemetry) RecordPatch(context.Context, string, fsops.PatchOutcomeEvent) {}
func (g *goldenTelemetry) RecordSearch(_ context.Context, id string, ev fsops.SearchOutcomeEvent) {
	g.add("telemetry search session=%s outcome=%s results=%v scale=%v queued=%v backend=%v large=%v", id, ev.Outcome, ev.Results, ev.Scale, ev.Queued, ev.Backend, ev.Large)
}
func (g *goldenTelemetry) RecordSearchFirstResult(_ context.Context, id string, ev fsops.SearchFirstResultEvent) {
	g.add("telemetry first session=%s scale=%v", id, ev.Scale)
}

type goldenFilesHandler struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	files *FileService
}

func (h goldenFilesHandler) SearchContent(ctx context.Context, req *connect.Request[sessionv1.SearchContentRequest], st *connect.ServerStream[sessionv1.SearchContentResponse]) error {
	return h.files.SearchContent(ctx, req, st)
}

func (h goldenFilesHandler) ReadFileRange(ctx context.Context, req *connect.Request[sessionv1.ReadFileRangeRequest]) (*connect.Response[sessionv1.ReadFileRangeResponse], error) {
	return h.files.ReadFileRange(ctx, req)
}

type goldenRig struct {
	t      *testing.T
	svc    *FileService
	ws     *goldenWorkspaces
	tel    *goldenTelemetry
	client sessionv1connect.SessionServiceClient
}

func goldenWriteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func goldenTree(t *testing.T) string {
	t.Helper()
	root, outside := t.TempDir(), t.TempDir()
	for rel, content := range map[string]string{
		"src/a.go":                "package a\n// needle one\nfunc Needle() {} // NEEDLE needle\n",
		"src/emoji.txt":           "\xEF\xBB\xBF😀 needle\r\nsecond needles\r\n",
		"src/long.txt":            strings.Repeat("x", 900) + "needle" + strings.Repeat("y", 900) + "\n",
		"src/bin.dat":             "needle\x00\x01\x02",
		"src/many.txt":            strings.Repeat("needle\n", 80),
		".gitignore":              "ignored/\n*.log\n",
		"sub/.gitignore":          "!keep.log\nlocal.txt\n",
		"sub/keep.log":            "needle kept\n",
		"sub/local.txt":           "needle local\n",
		"ignored/x.txt":           "needle ignored\n",
		"trace.log":               "needle log\n",
		"node_modules/m.js":       "needle node\n",
		"vendor/v.go":             "needle vendor\n",
		"dist/d.js":               "needle dist\n",
		"build/b.js":              "needle build\n",
		"target/t.js":             "needle target\n",
		".tox/t.py":               "needle tox\n",
		"__pycache__/p.py":        "needle pyc\n",
		".gradle/g.kts":           "needle gradle\n",
		".git/config":             "needle git\n",
		".a.txt.ssq-tmp-abc":      "needle temp\n",
		"huge.txt":                strings.Repeat("filler line of text\n", 130_000) + "needle past the default cap\n",
		"unicode/café needle.txt": "needle café\n",
	} {
		goldenWriteFile(t, root, rel, content)
	}
	goldenWriteFile(t, outside, "secret.txt", "needle outside the worktree\n")
	require.NoError(t, os.Symlink(filepath.Join(root, "src", "a.go"), filepath.Join(root, "inside_link.go")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "escape_link.txt")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape_dir")))
	require.NoError(t, os.Symlink(filepath.Join(root, "src"), filepath.Join(root, "inside_dir")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "src", ".gitignore")))
	return root
}

func goldenRegistry(on map[string]bool) *featureflags.Registry {
	specs := testFlagRegistry().Specs()
	for i := range specs {
		if v, ok := on[specs[i].Name]; ok {
			specs[i].Default = v
		}
	}
	return featureflags.NewRegistry(specs)
}

func newGoldenRig(t *testing.T) *goldenRig {
	t.Helper()
	root := goldenTree(t)
	disk := domain.Workspace{RepoRoot: root, ActiveDir: root, ExistingDir: root}
	ws := &goldenWorkspaces{byID: map[string]domain.Workspace{
		"s1":   disk,
		"gone": {RepoRoot: root, ActiveDir: filepath.Join(root, "removed-worktree"), ExistingDir: root},
	}}
	svc := NewFileService(ws)
	svc.SetFlagRegistry(goldenRegistry(map[string]bool{flagsvc.FilesViewerV2FlagName: true, flagsvc.FilesRangeReadFlagName: true}))
	tel := &goldenTelemetry{}
	svc.SetTelemetry(tel) // after construction: the search handler must read it lazily
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(goldenFilesHandler{files: svc}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &goldenRig{t: t, svc: svc, ws: ws, tel: tel, client: sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL)}
}

func goldenErr(err error) string {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return fmt.Sprint(err)
	}
	out := fmt.Sprintf("%s: %s", ce.Code(), ce.Message())
	for _, d := range ce.Details() {
		v, derr := d.Value()
		if derr != nil {
			continue
		}
		var b []byte
		if b, derr = (proto.MarshalOptions{Deterministic: true}).Marshal(v); derr == nil {
			out += fmt.Sprintf(" [%s %x]", d.Type(), b)
		}
	}
	return out
}

// search returns the order-independent description of one SearchContent call.
func (r *goldenRig) search(msg *sessionv1.SearchContentRequest) string {
	r.t.Helper()
	st, err := r.client.SearchContent(context.Background(), connect.NewRequest(msg))
	require.NoError(r.t, err)
	var lines []string
	var tail []string
	for st.Receive() {
		switch m := st.Msg().GetMessage().(type) {
		case *sessionv1.SearchContentResponse_Batch:
			for _, h := range m.Batch.GetMatches() {
				lines = append(lines, fmt.Sprintf("  %s:%d [%d,%d) @%d len=%d %.60q", h.Path, h.Line, h.StartUtf16, h.EndUtf16, h.ByteOffset, len(h.Text), h.Text))
			}
		case *sessionv1.SearchContentResponse_Stats:
			s := m.Stats
			tail = append(tail, fmt.Sprintf("  stats scanned=%d binary=%d truncated=%v large_skipped=%d", s.FilesScanned, s.BinarySkipped, s.Truncated, s.LargeFilesSkipped))
		case *sessionv1.SearchContentResponse_Queued:
			tail = append(tail, fmt.Sprintf("  queued ahead=%d", m.Queued.Ahead))
		}
	}
	sort.Strings(lines)
	if msg.GetOptions().GetLimits().GetMaxResults() > 0 {
		// Which matches survive a cap depends on worker order, and so does the scan count.
		lines = []string{fmt.Sprintf("  capped matches=%d", len(lines))}
		tail = nil
	}
	if err := st.Err(); err != nil {
		tail = append(tail, "  error "+goldenErr(err))
	}
	return strings.Join(append(lines, tail...), "\n")
}

func goldenSearchRequest(session, pattern string, edit func(*sessionv1.SearchContentRequest)) *sessionv1.SearchContentRequest {
	msg := &sessionv1.SearchContentRequest{SessionId: session, Options: &sessionv1.SearchOptions{Pattern: pattern}}
	if edit != nil {
		edit(msg)
	}
	return msg
}

func (r *goldenRig) recordSearches(out *bytes.Buffer) {
	scope := func(include, exclude []string, ignored bool) func(*sessionv1.SearchContentRequest) {
		return func(m *sessionv1.SearchContentRequest) {
			m.Options.Scope = &sessionv1.SearchScope{IncludeGlobs: include, ExcludeGlobs: exclude, IncludeIgnored: ignored}
		}
	}
	cases := []struct {
		name, session, pattern string
		edit                   func(*sessionv1.SearchContentRequest)
	}{
		{"literal", "s1", "needle", nil},
		{"include ignored", "s1", "needle", scope(nil, nil, true)},
		{"include glob", "s1", "needle", scope([]string{"**/*.go"}, nil, false)},
		{"exclude glob", "s1", "needle", scope(nil, []string{"src/**"}, false)},
		{"insensitive", "s1", "needle", func(m *sessionv1.SearchContentRequest) {
			m.Options.CaseMode = sessionv1.SearchCaseMode_SEARCH_CASE_MODE_INSENSITIVE
		}},
		{"whole word", "s1", "needle", func(m *sessionv1.SearchContentRequest) {
			m.Options.WordMode = sessionv1.SearchWordMode_SEARCH_WORD_MODE_WHOLE
		}},
		{"regex", "s1", "ne+dle (one|café)", func(m *sessionv1.SearchContentRequest) {
			m.Options.MatchMode = sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_REGEX
		}},
		{"large files", "s1", "needle past", func(m *sessionv1.SearchContentRequest) { m.Options.LargeFiles = true }},
		{"default cap skips large", "s1", "needle past", nil},
		{"result cap", "s1", "needle", func(m *sessionv1.SearchContentRequest) {
			m.Options.Limits = &sessionv1.SearchLimits{MaxResults: 5}
		}},
		{"too short", "s1", "n", nil},
		{"too long", "s1", strings.Repeat("a", 1025), nil},
		{"bad regex", "s1", "(", func(m *sessionv1.SearchContentRequest) {
			m.Options.MatchMode = sessionv1.SearchMatchMode_SEARCH_MATCH_MODE_REGEX
		}},
		{"bad glob", "s1", "needle", scope([]string{"["}, nil, false)},
		{"unknown session", "nope", "needle", nil},
		{"workspace failure", "broken", "needle", nil},
		{"worktree missing", "gone", "needle", nil},
	}
	for _, c := range cases {
		got := r.search(goldenSearchRequest(c.session, c.pattern, c.edit))
		tel := r.tel.drain()
		if c.name == "result cap" {
			tel = []string{fmt.Sprintf("telemetry events=%d", len(tel))}
		}
		fmt.Fprintf(out, "== search %s\n%s\n  %s\n", c.name, got, strings.Join(tel, "\n  "))
	}
}

func withoutQueuePosition(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "  queued ahead=") {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// recordBusy holds searches inside the workspace lookup, so their stream slots stay taken.
func (r *goldenRig) recordBusy(out *bytes.Buffer) {
	t := r.t
	release := r.ws.arm()
	defer release()
	held := make(chan string, 3)
	for i := range 3 {
		go func() {
			held <- r.search(goldenSearchRequest("s1-hold", "needle one", func(m *sessionv1.SearchContentRequest) { m.ClientId = fmt.Sprint("tab", i) }))
		}()
		r.ws.waitEntered(t, i+1)
	}
	fmt.Fprintf(out, "== fourth stream from one host\n%s\n", r.search(goldenSearchRequest("s1", "needle", nil)))
	superseding := make(chan string, 1)
	go func() {
		superseding <- r.search(goldenSearchRequest("s1-hold", "needle one", func(m *sessionv1.SearchContentRequest) { m.ClientId = "tab0" }))
	}()
	time.Sleep(200 * time.Millisecond) // the superseding call cancels tab0, then waits for its slot
	release()
	results := []string{withoutQueuePosition(<-held), withoutQueuePosition(<-held), withoutQueuePosition(<-held)}
	sort.Strings(results)
	fmt.Fprintf(out, "== held streams\n%s\n== superseding stream\n%s\n", strings.Join(results, "\n-\n"), withoutQueuePosition(<-superseding))
	tel := r.tel.drain()
	for i := range tel {
		tel[i] = strings.Replace(tel[i], "queued=true", "queued=false", 1)
	}
	sort.Strings(tel)
	fmt.Fprintf(out, "  %s\n", strings.Join(tel, "\n  "))
}

func (r *goldenRig) readRange(session string) error {
	_, err := r.client.ReadFileRange(context.Background(), connect.NewRequest(&sessionv1.ReadFileRangeRequest{SessionId: session, Path: "src/a.go", Anchor: &sessionv1.ReadFileRangeRequest_Lines{Lines: &sessionv1.LineAnchor{AnchorLine: 1, WantLines: 2}}}))
	return err
}

// recordRangeLimits fills the per-client then the per-IP range slots.
func (r *goldenRig) recordRangeLimits(out *bytes.Buffer) {
	t := r.t
	r.ws.byID["s2"], r.ws.byID["s3"] = r.ws.byID["s1"], r.ws.byID["s1"]
	release := r.ws.arm()
	defer release()
	var wg sync.WaitGroup
	hold := func(session string, n, total int) {
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				assert.NoError(t, r.readRange(session))
			}()
		}
		r.ws.waitEntered(t, total)
	}
	hold("s1-hold", 4, 4)
	fmt.Fprintf(out, "== fifth range from one client\n%s\n", goldenErr(r.readRange("s1-hold")))
	hold("s2-hold", 4, 8)
	fmt.Fprintf(out, "== ninth range from one ip\n%s\n", goldenErr(r.readRange("s3-hold")))
	release()
	wg.Wait()
	fmt.Fprintf(out, "== range after release\n%v\n", r.readRange("s1"))
}

func (r *goldenRig) recordListings(out *bytes.Buffer) {
	ctx := context.Background()
	for _, ignored := range []bool{false, true} {
		list, err := r.svc.ListFiles(ctx, connect.NewRequest(&sessionv1.ListFilesRequest{SessionId: "s1", Path: ".", IncludeIgnored: ignored}))
		require.NoError(r.t, err)
		var names []string
		for _, f := range list.Msg.Files {
			names = append(names, fmt.Sprintf("%s dir=%v ignored=%v", f.Name, f.IsDir, f.IsIgnored))
		}
		sort.Strings(names)
		fmt.Fprintf(out, "== list ignored=%v\n  %s\n", ignored, strings.Join(names, "\n  "))

		for _, query := range []string{".js", ".go", ".t", "eedle"} {
			found, err := r.svc.SearchFiles(ctx, connect.NewRequest(&sessionv1.SearchFilesRequest{SessionId: "s1", Query: query, IncludeIgnored: ignored}))
			require.NoError(r.t, err)
			names = names[:0]
			for _, f := range found.Msg.Files {
				names = append(names, f.Path)
			}
			sort.Strings(names)
			fmt.Fprintf(out, "== search files %q ignored=%v\n  %s\n", query, ignored, strings.Join(names, "\n  "))
		}
	}
	caps, err := r.svc.GetFileTransportCapabilities(ctx, connect.NewRequest(&sessionv1.GetFileTransportCapabilitiesRequest{}))
	require.NoError(r.t, err)
	limits := caps.Msg.GetCapabilities().GetLimits()
	fmt.Fprintf(out, "== limits hash_cap=%d max_ranges=%d\n", limits.GetHashCapBytes(), limits.GetMaxConcurrentRanges())
}

func TestContentSearchWiring_ShouldMatchRecordedGolden_When_DrivenThroughFileService(t *testing.T) {
	r := newGoldenRig(t)
	var out bytes.Buffer
	r.recordSearches(&out)
	r.recordBusy(&out)
	r.recordRangeLimits(&out)
	r.recordListings(&out)

	r.svc.SetFlagRegistry(goldenRegistry(map[string]bool{flagsvc.FilesViewerV2FlagName: false}))
	r.tel.drain()
	fmt.Fprintf(&out, "== viewer v2 off after rewire\n%s\n  %s\n", r.search(goldenSearchRequest("s1", "needle", nil)), strings.Join(r.tel.drain(), "\n  "))

	const path = "testdata/content_search_wiring.golden"
	if *updateContentSearchGolden {
		require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), out.String())
}
