package searchrpc

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/vc"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// The gzipped golden below was recorded from the pre-move handlers in server/services at
// origin/main; regenerate only with UPDATE_GOLDEN=1 and review the diff.
const goldenPath = "testdata/searchrpc.golden.gz"

// Package-specific seams (the pre-move recording used the unexported names).
func goldenVCS(s *vc.VCSStatus) *sessionv1.VCSStatus { return VCSStatusToProto(s) }
func goldenObserve(kind string, hit bool, d time.Duration) {
	ArtifactSearchStats.Observe(kind, hit, d)
}

var testDirSuffix = regexp.MustCompile(`TestSearchrpcGolden\d+`)

// A symlink entry's size is the length of its target path, which embeds the random temp dir.
var symlinkSize = regexp.MustCompile(`("name":"(?:dangling|linkfile|linkdir)","path":"[^"]*","is_dir":(?:true|false),"size":)\d+`)

// A directory entry's size is filesystem-specific.
var dirSize = regexp.MustCompile(`("is_dir":true,"size":)\d+`)

type goldenRec struct {
	sb       strings.Builder
	scrubber *strings.Replacer
}

func (g *goldenRec) section(name string) { fmt.Fprintf(&g.sb, "\n=== %s\n", name) }

func (g *goldenRec) line(format string, args ...any) {
	out := g.scrubber.Replace(fmt.Sprintf(format, args...))
	fmt.Fprintf(&g.sb, "%s\n", testDirSuffix.ReplaceAllString(out, "TestSearchrpcGolden<N>"))
}

func (g *goldenRec) msg(label string, m proto.Message) {
	raw, err := protojson.Marshal(m)
	if err != nil {
		g.line("%s: marshal error %v", label, err)
		return
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		g.line("%s: unmarshal error %v", label, err)
		return
	}
	out, _ := json.MarshalIndent(v, "", " ")
	g.line("%s: %s", label, out)
}

func (g *goldenRec) connectErr(label string, err error) {
	if err == nil {
		g.line("%s: ok", label)
		return
	}
	if ce := new(connect.Error); asConnect(err, &ce) {
		g.line("%s: code=%s msg=%s", label, ce.Code(), ce.Message())
		return
	}
	g.line("%s: plain error %v", label, err)
}

func asConnect(err error, target **connect.Error) bool {
	ce, ok := err.(*connect.Error)
	if ok {
		*target = ce
	}
	return ok
}

func TestSearchrpcGolden(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seedClaudeHome(t, goldenSessions(base))
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	root := t.TempDir()
	tmpEnv := os.Getenv("TMPDIR")
	pairs := []string{root, "<ROOT>", home, "<HOME>"}
	if tmpEnv != "" {
		pairs = append(pairs, tmpEnv, "<TMPDIR>")
	}
	g := &goldenRec{scrubber: strings.NewReplacer(pairs...)}

	goldenSearch(t, g)
	goldenPathMatching(g, root, home)
	goldenLocalFiles(t, g, root)
	goldenVCSProto(g)
	goldenMetrics(g)

	got := g.sb.String()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		require.NoError(t, err)
		_, err = zw.Write([]byte(got))
		require.NoError(t, err)
		require.NoError(t, zw.Close())
		require.NoError(t, os.WriteFile(goldenPath, buf.Bytes(), 0o644))
		return
	}
	f, err := os.Open(goldenPath)
	require.NoError(t, err)
	defer f.Close()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	want, err := io.ReadAll(zr)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func goldenSessions(base time.Time) map[string]testConvSession {
	// Every message gets a distinct number of filler tokens so no two matches tie on
	// BM25 score: the engine breaks ties in map order, which would make the golden flaky.
	pad := 0
	mk := func(project string, texts ...string) testConvSession {
		s := testConvSession{project: project}
		for i, tx := range texts {
			role := "user"
			if i%2 == 1 {
				role = "assistant"
			}
			pad++
			for j := 0; j < pad; j++ {
				tx += fmt.Sprintf(" zq%dx%d", pad, j)
			}
			s.messages = append(s.messages, testConvMsg{role: role, content: tx, time: base.Add(time.Duration(pad) * time.Minute)})
		}
		return s
	}
	return map[string]testConvSession{
		"sess-auth":    mk("/repo/a", "auth refactor plan for login", "sounds good, auth tokens next", "refactor the auth middleware", "done with the auth middleware"),
		"sess-dark":    mk("/repo/a", "dark mode toggle setting", "toggle added", "dark mode toggle persists", "ok", "dark mode toggle in settings page"),
		"sess-b":       mk("/repo/b", "database migration for sessions", "migration written", "run the migration now"),
		"sess-unicode": mk("/repo/c", "naïve café résumé search", "日本語 のテスト search", "emoji 🚀 launch search"),
		"sess-special": mk("/repo/c", "regex [a-z]+ (group) *star* \"quoted\" it's", "backslash \\ and percent % and $VAR", "tab\tand newline-ish text"),
		"sess-long":    mk("/repo/d", strings.Repeat("alpha beta gamma delta ", 40), "alpha only", "gamma only"),
	}
}

func goldenSearch(t *testing.T, g *goldenRec) {
	svc := setupSyncedSearchService(t)
	ctx := t.Context()

	g.section("search")
	queries := []string{
		"auth", "auth refactor", "dark mode toggle", "migration", "search", "alpha", "gamma delta",
		"café", "日本語", "🚀", "regex [a-z]+", "(group)", "*star*", "\"quoted\"", "it's", "$VAR", "\\", "%",
		"nonexistentterm", "", "   ", "the", "a", "AUTH", "Auth Middleware", strings.Repeat("x", 300),
	}
	type opt struct {
		name string
		mod  func(*sessionv1.SearchClaudeHistoryRequest)
	}
	opts := []opt{
		{"default", func(r *sessionv1.SearchClaudeHistoryRequest) {}},
		{"limit2", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Limit = 2 }},
		{"limit2-offset1", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Limit = 2; r.Offset = 1 }},
		{"group", func(r *sessionv1.SearchClaudeHistoryRequest) { r.GroupBySession = ptr(true); r.Limit = 5 }},
		{"group-ctx", func(r *sessionv1.SearchClaudeHistoryRequest) {
			r.GroupBySession = ptr(true)
			r.IncludeContext = ptr(true)
			r.Limit = 5
		}},
		{"project-a", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Project = ptr("/repo/a") }},
		{"project-none", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Project = ptr("/nope") }},
		{"limit-over-max", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Limit = 1000 }},
		{"limit-negative", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Limit = -3 }},
		{"offset-with-group", func(r *sessionv1.SearchClaudeHistoryRequest) { r.Offset = 1; r.GroupBySession = ptr(true) }},
		{"exclude-automation", func(r *sessionv1.SearchClaudeHistoryRequest) { r.ExcludeAutomationSessions = ptr(true) }},
	}
	// The engine breaks BM25 score ties in map order, so a query whose plain result list has
	// tied scores is recorded only in canonical order and only for variants that do not
	// depend on which tied hit survives paging or session grouping.
	tieSafe := map[string]bool{"default": true, "project-a": true, "project-none": true, "limit-over-max": true, "exclude-automation": true}
	for _, q := range queries {
		tied := false
		if plain, err := svc.SearchClaudeHistory(ctx, connect.NewRequest(&sessionv1.SearchClaudeHistoryRequest{Query: q, Limit: 100})); err == nil {
			rs := plain.Msg.Results
			for i := 1; i < len(rs); i++ {
				if rs[i].Score == rs[i-1].Score {
					tied = true
				}
			}
		}
		for _, o := range opts {
			req := &sessionv1.SearchClaudeHistoryRequest{Query: q}
			o.mod(req)
			label := fmt.Sprintf("search q=%q opt=%s", q, o.name)
			if tied && !tieSafe[o.name] {
				g.line("%s: skipped (tied scores)", label)
				continue
			}
			resp, err := svc.SearchClaudeHistory(ctx, connect.NewRequest(req))
			if err != nil {
				g.connectErr(label, err)
				continue
			}
			if tied {
				sort.SliceStable(resp.Msg.Results, func(i, j int) bool {
					a, b := resp.Msg.Results[i], resp.Msg.Results[j]
					if a.Score != b.Score {
						return a.Score > b.Score
					}
					if a.SessionId != b.SessionId {
						return a.SessionId < b.SessionId
					}
					return a.MessageIndex < b.MessageIndex
				})
			}
			g.msg(label, resp.Msg)
		}
	}

	g.section("list")
	for _, req := range []*sessionv1.ListClaudeHistoryRequest{
		{},
		{Limit: 2},
		{PageSize: 2},
		{PageSize: 1000},
		{Project: ptr("/repo/a")},
		{SearchQuery: ptr("auth")},
		{SearchQuery: ptr("café")},
		{PageToken: "garbage"},
		{ExcludeAutomationSessions: ptr(true)},
	} {
		resp, err := svc.ListClaudeHistory(ctx, connect.NewRequest(req))
		label := fmt.Sprintf("list proj=%q q=%q limit=%d size=%d tok=%q excl=%v", req.GetProject(), req.GetSearchQuery(), req.Limit, req.PageSize, req.PageToken, req.GetExcludeAutomationSessions())
		if err != nil {
			g.connectErr(label, err)
			continue
		}
		g.msg(label, resp.Msg)
		// Follow the cursor for the page-size cases.
		tok := resp.Msg.GetNextPageToken()
		for i := 0; tok != "" && i < 10; i++ {
			next := &sessionv1.ListClaudeHistoryRequest{PageSize: req.PageSize, Limit: req.Limit, PageToken: tok}
			r2, err := svc.ListClaudeHistory(ctx, connect.NewRequest(next))
			if err != nil {
				g.connectErr(label+" next", err)
				break
			}
			g.msg(fmt.Sprintf("%s next%d", label, i), r2.Msg)
			tok = r2.Msg.GetNextPageToken()
		}
	}

	g.section("messages-and-detail")
	for _, id := range []string{"sess-auth", "sess-dark", "sess-unicode", "sess-long", "", "missing"} {
		for _, req := range []*sessionv1.GetClaudeHistoryMessagesRequest{
			{Id: id},
			{Id: id, Limit: 2},
			{Id: id, Limit: 2, Offset: 1},
			{Id: id, Limit: 2, Tail: true},
			{Id: id, Limit: 3, AnchorIndex: ptr(int32(2))},
			{Id: id, Limit: 2, Offset: 1, Tail: true},
			{Id: id, Limit: -1},
		} {
			resp, err := svc.GetClaudeHistoryMessages(ctx, connect.NewRequest(req))
			label := fmt.Sprintf("messages id=%q limit=%d offset=%d tail=%v anchor=%v", req.Id, req.Limit, req.Offset, req.Tail, req.GetAnchorIndex())
			if err != nil {
				g.connectErr(label, err)
				continue
			}
			g.msg(label, resp.Msg)
		}
		resp, err := svc.GetClaudeHistoryDetail(ctx, connect.NewRequest(&sessionv1.GetClaudeHistoryDetailRequest{Id: id}))
		if err != nil {
			g.connectErr("detail "+id, err)
			continue
		}
		g.msg("detail "+id, resp.Msg)
	}
}

func goldenPathMatching(g *goldenRec, root, home string) {
	g.section("path-matching")
	ctx := classifier.ClassificationContext{Cwd: filepath.Join(root, "work"), RepoRoot: filepath.Join(root, "work", "repo")}
	args := []string{
		"/", "//", "///", "/.", "/..", "/../..", "~", "~/", "~/x", "~/../x", "~root", "~/..", home, home + "/", home + "/..",
		"/etc", "/etc/", "/etc/passwd", "/etcetera", "/etc/../tmp", "/usr/bin/../lib", "/proc/self", "/dev/null", "/run/user",
		"/lib64/x", "/boot", "/sys/kernel", "/bin", "/sbin", "/usr",
		"/tmp", "/tmp/", "/tmp/../etc", "/tmpx", "/var/tmp/x", "/var/tmpx", "$TMPDIR", "${TMPDIR}/x", "$HOME", "${HOME}/x", "$HOME/../x", "$NOPE", "${NOPE}/x",
		"relative", "./relative", "../up", "..", ".", "", " ", "-f", "-rf", "--", "--no-preserve-root", "-",
		ctx.Cwd, ctx.Cwd + "/", ctx.Cwd + "/sub", ctx.Cwd + "/../x", ctx.Cwd + "x", ctx.RepoRoot, ctx.RepoRoot + "/f", ctx.RepoRoot + "/../f",
		"/a//b/../c/./d", "/a/b/c/../../..", "/\x00nul", "/with space/x", "/uni/日本語", "~/日本語", strings.Repeat("/a", 200),
		"file:///etc/passwd", "C:\\Windows", "\\\\server\\share", "/etc\n/tmp",
	}
	for _, a := range args {
		exp := ExpandPath(a)
		g.line("expand %q -> %q class=%d", a, exp, ClassifyPath(exp, ctx))
		g.line("classify-raw %q class=%d", a, ClassifyPath(a, ctx))
	}
	empty := classifier.ClassificationContext{}
	for _, a := range args {
		g.line("classify-emptyctx %q class=%d", a, ClassifyPath(a, empty))
	}

	flags := []PathClassification{0, PathRoot, PathHome, PathSystemDir, PathTempDir, PathCwd, PathGitRepo,
		PathRoot | PathHome, PathSystemDir | PathTempDir, PathCwd | PathGitRepo, PathRoot | PathHome | PathSystemDir | PathTempDir | PathCwd | PathGitRepo}
	argSets := [][]string{
		nil, {}, {"-rf"}, {"-rf", "/"}, {"/", "-rf"}, {"/tmp/x", "/etc"}, {"/etc", "/tmp/x"}, {"~", "/tmp"}, {ctx.Cwd + "/a", "-v", ctx.RepoRoot + "/b"},
		{"-a", "-b"}, {"", ""}, {"/", "/", "/"}, {"relative", "/etc"},
	}
	var rows []string
	for _, idx := range []int{-5, -1, 0, 1, 2, 7} {
		for _, m := range flags {
			for _, r := range flags {
				pm := &PathMatcher{ArgIndex: idx, MatchIf: m, RejectIf: r}
				for i, as := range argSets {
					expanded := make([]string, len(as))
					for j, a := range as {
						expanded[j] = ExpandPath(a)
					}
					rows = append(rows, fmt.Sprintf("match idx=%d matchIf=%d rejectIf=%d args#%d=%v ctx=with -> %v ctx=empty -> %v",
						idx, m, r, i, as, pm.Matches(expanded, ctx), pm.Matches(expanded, empty)))
				}
			}
		}
	}
	sort.Strings(rows)
	for _, r := range rows {
		g.line("%s", r)
	}
	g.line("nonFlagArgs %q %q %q", nonFlagArgs(nil), nonFlagArgs([]string{"-a", "b", "--c", "-", "d"}), nonFlagArgs([]string{"", "-"}))
}

func goldenLocalFiles(t *testing.T, g *goldenRec, root string) {
	g.section("local-files")
	tree := filepath.Join(root, "tree")
	mt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	mk := func(rel string, data string) {
		p := filepath.Join(tree, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(data), 0o644))
		require.NoError(t, os.Chtimes(p, mt, mt))
	}
	mk("b.txt", "bee")
	mk("a.txt", "ay")
	mk("Z.txt", "zed")
	mk(".hidden", "h")
	mk("dir2/inner.txt", "in2")
	mk("dir1/inner.txt", "in1")
	mk("日本語.txt", "jp")
	mk("with space.txt", "sp")
	mk("big/f.bin", strings.Repeat("0123456789", 100))
	require.NoError(t, os.Symlink(filepath.Join(tree, "dir1"), filepath.Join(tree, "linkdir")))
	require.NoError(t, os.Symlink(filepath.Join(tree, "a.txt"), filepath.Join(tree, "linkfile")))
	require.NoError(t, os.Symlink(filepath.Join(tree, "gone"), filepath.Join(tree, "dangling")))
	ts := unix.NsecToTimespec(mt.UnixNano())
	for _, l := range []string{"linkdir", "linkfile", "dangling"} {
		require.NoError(t, unix.UtimesNanoAt(unix.AT_FDCWD, filepath.Join(tree, l), []unix.Timespec{ts, ts}, unix.AT_SYMLINK_NOFOLLOW))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "empty"), 0o755))
	// The listing cap is 2000 entries: cover below, at and above it.
	for dir, n := range map[string]int{"many": 2003, "exact": 2000, "under": 1999} {
		require.NoError(t, os.MkdirAll(filepath.Join(tree, dir), 0o755))
		for i := 0; i < n; i++ {
			p := filepath.Join(tree, dir, fmt.Sprintf("f%05d", i))
			require.NoError(t, os.WriteFile(p, nil, 0o644))
			require.NoError(t, os.Chtimes(p, mt, mt))
		}
	}
	for _, d := range []string{"", "dir1", "dir2", "empty", "big", "many", "exact", "under", "linkdir"} {
		require.NoError(t, os.Chtimes(filepath.Join(tree, d), mt, mt))
	}
	// Entry directory mtimes vary by filesystem; entries sorted dirs-first, then name.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(filepath.Join(home, ".claude"), mt, mt))
	svc := NewLocalFileService()

	paths := []string{
		tree, tree + "/", tree + "/dir1", tree + "/dir1/../dir2", tree + "/../tree", tree + "/nope", tree + "/a.txt",
		tree + "/linkdir", tree + "/dangling", tree + "/empty", tree + "/many", tree + "/exact", tree + "/under", tree + "//dir1", "relative", "./x", "..",
		tree + "/dir1/inner.txt/", "/proc/definitely/not/here", "", tree + "/\x00bad",
	}
	for _, p := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead} {
			rec := httptest.NewRecorder()
			q := "/api/local/files/list?path=" + urlEscape(p)
			svc.ListLocalDirectory(rec, httptest.NewRequest(method, q, nil))
			g.recorder(fmt.Sprintf("list %s path=%q", method, p), rec, true)
		}
	}
	rec := httptest.NewRecorder()
	svc.ListLocalDirectory(rec, httptest.NewRequest(http.MethodGet, "/api/local/files/list", nil))
	g.recorder("list default-home", rec, true)

	for _, p := range []string{
		tree + "/a.txt", tree + "/b.txt", tree + "/日本語.txt", tree + "/with space.txt", tree + "/linkfile", tree + "/linkdir", tree + "/dangling",
		tree + "/dir1", tree, tree + "/nope", tree + "/../tree/a.txt", tree + "//a.txt", "relative/a.txt", "a.txt", "/", "/.hidden", tree + "/.hidden",
		tree + "/big/f.bin", tree + "/empty",
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(method, "http://x/", nil)
			req.URL.Path = p
			svc.ServeLocalFile(rec, req)
			g.recorder(fmt.Sprintf("serve %s path=%q", method, p), rec, false)
		}
	}
	rangeReq := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	rangeReq.URL.Path = tree + "/big/f.bin"
	rangeReq.Header.Set("Range", "bytes=10-19")
	rec = httptest.NewRecorder()
	svc.ServeLocalFile(rec, rangeReq)
	g.recorder("serve range", rec, false)
}

func urlEscape(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '/' || b == '.' || b == '-' || b == '_' {
			sb.WriteByte(b)
			continue
		}
		fmt.Fprintf(&sb, "%%%02X", b)
	}
	return sb.String()
}

func (g *goldenRec) recorder(label string, rec *httptest.ResponseRecorder, json bool) {
	res := rec.Result()
	defer res.Body.Close()
	keys := make([]string, 0, len(res.Header))
	for k := range res.Header {
		if k == "Last-Modified" || k == "Date" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var hdr []string
	for _, k := range keys {
		hdr = append(hdr, k+"="+strings.Join(res.Header[k], ","))
	}
	body := testDirSuffix.ReplaceAllString(g.scrubber.Replace(rec.Body.String()), "TestSearchrpcGolden<N>")
	body = symlinkSize.ReplaceAllString(body, "${1}0")
	body = dirSize.ReplaceAllString(body, "${1}0")
	if json {
		body = strings.ReplaceAll(body, "\n", "")
		if len(body) > 600 {
			body = body[:300] + "..." + body[len(body)-300:]
		}
	} else if len(body) > 200 {
		body = fmt.Sprintf("%s...(%d bytes)", body[:200], len(body))
	}
	g.line("%s -> %d [%s] %q", label, res.StatusCode, strings.Join(hdr, ";"), body)
}

func goldenVCSProto(g *goldenRec) {
	g.section("vcs-proto")
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	g.line("nil -> %v", goldenVCS(nil))
	statuses := []*vc.VCSStatus{
		{},
		{Type: vc.VCSGit, Branch: "main", HeadCommit: "abc", Description: "d", AheadBy: 3, BehindBy: 4, Upstream: "origin/main",
			HasStaged: true, HasUnstaged: true, HasUntracked: true, HasConflicts: true, IsClean: false,
			StagedFiles:      []vc.FileChange{{Path: "a", Status: vc.FileModified, IsStaged: true, Additions: 1, Deletions: 2}},
			UnstagedFiles:    []vc.FileChange{{Path: "b", Status: vc.FileAdded}, {Path: "c", OldPath: "c0", Status: vc.FileRenamed}},
			UntrackedFiles:   []vc.FileChange{{Path: "d", Status: vc.FileUntracked}, {Path: "e", Status: vc.FileIgnored}},
			ConflictFiles:    []vc.FileChange{{Path: "f", Status: vc.FileConflict}, {Path: "g", Status: vc.FileCopied}, {Path: "h", Status: vc.FileDeleted}, {Path: "i", Status: vc.FileStatus(99)}},
			Commits:          []git.ShippedCommit{{SHA: "1", Summary: "s", AuthorName: "n", AuthorAt: at}, {SHA: "2"}},
			CommitsTruncated: true, CommitsUnavailable: true,
			AggregateDiffStat: &git.AggregateDiffStat{FilesChanged: 5, Additions: 6, Deletions: 7}, StatusAsOf: at},
		{Type: vc.VCSJujutsu, IsClean: true},
		{Type: vc.VCSType(77)},
	}
	for i, s := range statuses {
		g.msg(fmt.Sprintf("status#%d", i), goldenVCS(s))
	}
}

func goldenMetrics(g *goldenRec) {
	g.section("metrics")
	kinds := []string{"text", "pr", "sha", "branch", "weird kind", "never"}
	before := map[string][3]int64{}
	for _, k := range kinds {
		before[k] = [3]int64{ArtifactSearchQueryCount(k, "hit"), ArtifactSearchQueryCount(k, "miss"), ArtifactSearchQueryCount(k, "x")}
	}
	for _, k := range kinds[:5] {
		goldenObserve(k, true, time.Millisecond)
		goldenObserve(k, true, time.Second)
		goldenObserve(k, false, 0)
	}
	for _, k := range kinds {
		b := before[k]
		g.line("count kind=%q hit=%d miss=%d other=%d", k, ArtifactSearchQueryCount(k, "hit")-b[0], ArtifactSearchQueryCount(k, "miss")-b[1], ArtifactSearchQueryCount(k, "x")-b[2])
	}
	g.line("names %s %s", MetricArtifactSearchSeconds, MetricArtifactSearchQueries)
}
