package guardpaths

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tstapler/stapler-squad/internal/reporoot"
)

func repoFS(t *testing.T) fs.FS {
	t.Helper()
	root, err := reporoot.Find()
	if err != nil {
		t.Fatal(err)
	}
	return os.DirFS(root)
}

func requireNone(t *testing.T, problems []string) {
	t.Helper()
	for _, p := range problems {
		t.Error(p)
	}
}

func requireOne(t *testing.T, problems []string, want string) {
	t.Helper()
	if len(problems) != 1 || !strings.Contains(problems[0], want) {
		t.Fatalf("problems = %q, want exactly one containing %q", problems, want)
	}
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func TestRepo_NoArchivedRevivalFilesExist(t *testing.T) {
	t.Parallel()
	requireNone(t, CheckArchivedRevivalFiles(repoFS(t)))
}

func TestRepo_LiveInstanceRawSymbolsDeclaredInScope(t *testing.T) {
	t.Parallel()
	requireNone(t, CheckLiveInstanceRawScope(repoFS(t)))
}

func TestRepo_GoTestBzlRowsHaveTests(t *testing.T) {
	t.Parallel()
	requireNone(t, CheckGoTestBzlRows(repoFS(t)))
}

func TestRepo_CompareAffectedTargetsExist(t *testing.T) {
	t.Parallel()
	requireNone(t, CheckCompareAffectedTargets(repoFS(t)))
}

func TestRepo_SvctestMainCoversDriverStartingLeaves(t *testing.T) {
	t.Parallel()
	requireNone(t, CheckSvctestMain(repoFS(t)))
}

func TestCheckArchivedRevivalFiles_ReportsMissingPath(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		archivalAnalyzer: file("package a\nvar lifecycleFiles = []string{\"a/here.go\", \"a/gone.go\"}\n"),
		"a/here.go":      file("package a"),
	}
	requireOne(t, CheckArchivedRevivalFiles(fsys), "a/gone.go")
}

func TestCheckArchivedRevivalFiles_FailsWhenListUnparseable(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{archivalAnalyzer: file("package a\n")}
	requireOne(t, CheckArchivedRevivalFiles(fsys), "no lifecycleFiles literal")
}

func TestCheckLiveInstanceRawScope_ReportsMovedMethod(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		liveRawAnalyzer:             file("package a\nvar monitoredFuncNames = map[string]bool{\"IsSessionLive\": true, \"Gone\": true}\n"),
		"server/services/leaf/x.go": file("package leaf\nfunc (s *SessionService) IsSessionLive() {}\nfunc (s *SessionService) FindLiveInstance() {}\n"),
	}
	requireOne(t, CheckLiveInstanceRawScope(fsys), "method Gone")
}

func TestCheckGoTestBzlRows_ReportsRowWithoutTests(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		goTestBzl:                   file("_SIZES = {\n    \"server/services\": (\"enormous\", 4),\n    \"old/pkg\": (\"large\", 2),\n}\n"),
		"server/services/a_test.go": file("package services"),
	}
	requireOne(t, CheckGoTestBzlRows(fsys), "old/pkg")
}

func TestCheckCompareAffectedTargets_ReportsMissingDir(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		compareAffected:   file("package c\nvar _ = sel.hasTargetIn(\"//server/mcp\", \"//server/gone\")\n"),
		"server/mcp/x.go": file("package mcp"),
	}
	requireOne(t, CheckCompareAffectedTargets(fsys), "//server/gone")
}

func TestCheckSvctestMain_ReportsLeafWithoutMain(t *testing.T) {
	t.Parallel()
	const imp = "package p\nimport _ \"github.com/tstapler/stapler-squad/session/driver\"\n"
	fsys := fstest.MapFS{
		"server/services/withmain/a.go":              file(imp),
		"server/services/withmain/testmain_test.go":  file("package p\nfunc TestMain(m *testing.M) { svctest.Main(m) }\n"),
		"server/services/nomain/a.go":                file(imp),
		"server/services/nomain/a_test.go":           file("package p"),
		"server/services/internal/svctest/driver.go": file(imp),
	}
	requireOne(t, CheckSvctestMain(fsys), "server/services/nomain")
}

func TestCheckLiveInstanceRawScope_IgnoresMocksInSvctest(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		liveRawAnalyzer:                         file("package a\nvar monitoredFuncNames = map[string]bool{\"IsSessionLive\": true}\n"),
		"server/services/internal/svctest/m.go": file("package svctest\nfunc (m *MockStopper) IsSessionLive() {}\nfunc (m *Stub) FindLiveInstance() {}\n"),
		"server/services/other.go":              file("package services\nfunc (m *Unrelated) IsSessionLive() {}\n"),
	}
	problems := CheckLiveInstanceRawScope(fsys)
	if len(problems) != 2 {
		t.Fatalf("want IsSessionLive and FindLiveInstance reported, got %q", problems)
	}
}

func TestCheckSvctestMain_IgnoresCommentedImportAndCall(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"server/services/a/a.go":          file("package a\n// import \"github.com/tstapler/stapler-squad/session/driver\"\n"),
		"server/services/b/b.go":          file("package b\nimport _ \"github.com/tstapler/stapler-squad/session/driver\"\n"),
		"server/services/b/m_test.go":     file("package b\n// svctest.Main(m) TODO\n"),
		"server/services/testdata/c/c.go": file("package c\nimport _ \"github.com/tstapler/stapler-squad/session/driver\"\n"),
	}
	requireOne(t, CheckSvctestMain(fsys), "server/services/b")
	requireOne(t, CheckSvctestMain(fsys), "direct imports only")
}
