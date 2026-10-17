package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// fakeWorkspaceProvider implements WorkspaceProvider for tests.
type fakeWorkspaceProvider struct {
	existingDir string
}

func (f *fakeWorkspaceProvider) GetWorkspace(sessionID string) (session.Workspace, error) {
	if sessionID != "test-session" {
		return session.Workspace{}, connect.NewError(connect.CodeNotFound, nil)
	}
	return session.Workspace{ExistingDir: f.existingDir}, nil
}

// testFileService wraps FileService with a fake findInstance for unit tests.
// It bypasses storage and injects a session instance with a known path.
type testFileService struct {
	FileService
	testRoot string
}

func newTestFileService(root string) *testFileService {
	return &testFileService{
		FileService: FileService{workspace: nil},
		testRoot:    root,
	}
}

// findInstance overrides the base findInstance to use testRoot.
func (t *testFileService) findInstance(id string) (*session.Instance, error) {
	if id != "test-session" {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return &session.Instance{Title: "test-session", Path: t.testRoot}, nil
}

// listFilesTest is a testable version of ListFiles that uses the overridden findInstance.
func (t *testFileService) listFiles(ctx context.Context, req *connect.Request[sessionv1.ListFilesRequest]) (*connect.Response[sessionv1.ListFilesResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	inst, err := t.findInstance(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	basePath := inst.Path
	requestedPath := req.Msg.Path
	if requestedPath == "" {
		requestedPath = "."
	}

	fullPath, pathErr := resolveAndValidatePath(basePath, requestedPath)
	if pathErr != nil {
		return nil, pathErr
	}

	entries, readErr := os.ReadDir(fullPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil, connect.NewError(connect.CodeNotFound, nil)
		}
		return nil, connect.NewError(connect.CodeInternal, readErr)
	}

	var matcher gitignore.Matcher
	if !req.Msg.IncludeIgnored {
		patterns := loadGitignorePatterns(basePath, fullPath)
		matcher = gitignore.NewMatcher(patterns)
	}

	var dirs []*sessionv1.FileNode
	var files []*sessionv1.FileNode
	truncated := false
	totalCount := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && hardSkipDirs[name] {
			continue
		}

		isSymlink := entry.Type()&os.ModeSymlink != 0
		isDir := entry.IsDir()
		if isSymlink {
			isDir = false
		}

		entryFullPath := filepath.Join(fullPath, name)
		relPath, _ := filepath.Rel(basePath, entryFullPath)
		relSegments := strings.Split(filepath.ToSlash(relPath), "/")

		isIgnored := false
		if matcher != nil {
			isIgnored = matcher.Match(relSegments, isDir)
		}
		if isIgnored && !req.Msg.IncludeIgnored {
			continue
		}

		var size int64
		if !isDir && !isSymlink {
			if info, statErr := entry.Info(); statErr == nil {
				size = info.Size()
			}
		}

		node := &sessionv1.FileNode{
			Name:      name,
			Path:      filepath.ToSlash(relPath),
			IsDir:     isDir,
			Size:      size,
			IsIgnored: isIgnored,
			IsSymlink: isSymlink,
		}

		totalCount++
		if totalCount > maxDirEntries {
			truncated = true
			break
		}

		if isDir {
			dirs = append(dirs, node)
		} else {
			files = append(files, node)
		}
	}

	allNodes := append(dirs, files...)
	return connect.NewResponse(&sessionv1.ListFilesResponse{
		Files:      allNodes,
		BasePath:   requestedPath,
		Truncated:  truncated,
		TotalCount: int32(totalCount),
	}), nil
}

// getFileContentTest is a testable version of GetFileContent.
func (t *testFileService) getFileContent(ctx context.Context, req *connect.Request[sessionv1.GetFileContentRequest]) (*connect.Response[sessionv1.GetFileContentResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}

	inst, err := t.findInstance(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	fullPath, pathErr := resolveAndValidatePath(inst.Path, req.Msg.Path)
	if pathErr != nil {
		return nil, pathErr
	}

	info, statErr := os.Lstat(fullPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, connect.NewError(connect.CodeNotFound, nil)
		}
		return nil, connect.NewError(connect.CodeInternal, statErr)
	}

	size := info.Size()
	if size > maxFileSize {
		return nil, connect.NewError(connect.CodeFailedPrecondition, nil)
	}

	readLimit := size
	isTruncated := false
	if size > truncateSize {
		readLimit = truncateSize
		isTruncated = true
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	isKnownText := knownTextExtensions[ext]

	f, openErr := os.Open(fullPath)
	if openErr != nil {
		if os.IsNotExist(openErr) {
			return nil, connect.NewError(connect.CodeNotFound, nil)
		}
		return nil, connect.NewError(connect.CodeInternal, openErr)
	}
	defer func() { _ = f.Close() }()

	sniffBuf := make([]byte, 512)
	sniffN, _ := f.Read(sniffBuf)
	sniffBuf = sniffBuf[:sniffN]

	isBinary := false
	if !isKnownText {
		// Null-byte scan.
		for _, b := range sniffBuf {
			if b == 0 {
				isBinary = true
				break
			}
		}
	}

	if isBinary {
		return connect.NewResponse(&sessionv1.GetFileContentResponse{
			IsBinary: true,
			Size:     size,
		}), nil
	}

	_, _ = f.Seek(0, 0)
	buf := make([]byte, readLimit)
	n, _ := readFull(f, buf)
	buf = buf[:n]

	return connect.NewResponse(&sessionv1.GetFileContentResponse{
		Content:     string(buf),
		Encoding:    "utf-8",
		Size:        size,
		IsTruncated: isTruncated,
	}), nil
}

// searchFilesTest is a testable version of SearchFiles using testRoot directly.
func (t *testFileService) searchFiles(ctx context.Context, req *connect.Request[sessionv1.SearchFilesRequest]) (*connect.Response[sessionv1.SearchFilesResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}
	if _, err := t.findInstance(req.Msg.SessionId); err != nil {
		return nil, err
	}
	if len(req.Msg.Query) < 2 {
		return connect.NewResponse(&sessionv1.SearchFilesResponse{}), nil
	}
	maxResults := int(req.Msg.MaxResults)
	if maxResults <= 0 {
		maxResults = maxSearchResults
	}
	files, truncated, totalMatches, err := searchFilesInWorktree(ctx, t.testRoot, req.Msg.Query, req.Msg.IncludeIgnored, maxResults, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.SearchFilesResponse{
		Files:        files,
		Truncated:    truncated,
		TotalMatches: totalMatches,
	}), nil
}

// ---- Tests for resolveAndValidatePath ----

func TestResolveAndValidatePath_TraversalRejected(t *testing.T) {
	t.Parallel()
	base := t.TempDir()

	cases := []struct {
		rel  string
		desc string
	}{
		{"../etc/passwd", "simple traversal"},
		{"../../etc", "double traversal"},
		{"foo/../../etc/passwd", "traversal via subdir"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			_, err := resolveAndValidatePath(base, tc.rel)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.rel)
			}
		})
	}
}

func TestResolveAndValidatePath_ValidPath(t *testing.T) {
	t.Parallel()
	base := t.TempDir()

	cases := []string{".", "subdir", "subdir/file.go", "a/b/c"}
	for _, rel := range cases {
		_, err := resolveAndValidatePath(base, rel)
		if err != nil {
			t.Errorf("unexpected error for valid path %q: %v", rel, err)
		}
	}
}

// ---- Tests for ListFiles ----

func TestListFiles_HardSkipDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	for _, dir := range []string{"node_modules", "vendor", ".git", "__pycache__"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId: "test-session",
		Path:      ".",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names := make(map[string]bool)
	for _, node := range resp.Msg.Files {
		names[node.Name] = true
	}

	for _, skip := range []string{"node_modules", "vendor", ".git", "__pycache__"} {
		if names[skip] {
			t.Errorf("expected %q to be skipped, but it appeared in results", skip)
		}
	}
	if !names["src"] {
		t.Error("expected src/ to appear in results")
	}
	if !names["main.go"] {
		t.Error("expected main.go to appear in results")
	}
}

func TestListFiles_GitignoreFiltering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temp.tmp"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)

	// Without include_ignored: *.tmp should not appear.
	resp, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId:      "test-session",
		Path:           ".",
		IncludeIgnored: false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, node := range resp.Msg.Files {
		if strings.HasSuffix(node.Name, ".tmp") {
			t.Errorf("expected .tmp files to be filtered, got %q", node.Name)
		}
	}

	// With include_ignored: *.tmp should appear.
	resp2, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId:      "test-session",
		Path:           ".",
		IncludeIgnored: true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, node := range resp2.Msg.Files {
		if node.Name == "temp.tmp" {
			found = true
		}
	}
	if !found {
		t.Error("expected temp.tmp to appear when include_ignored=true")
	}
}

func TestListFiles_DirectoriesFirst(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "zdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "adir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "afile.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "zfile.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId: "test-session",
		Path:      ".",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodes := resp.Msg.Files
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
	if !nodes[0].IsDir || !nodes[1].IsDir {
		t.Error("expected first two entries to be directories")
	}
	if nodes[0].Name != "adir" {
		t.Errorf("expected first dir adir, got %q", nodes[0].Name)
	}
	if nodes[2].Name != "afile.go" {
		t.Errorf("expected first file afile.go, got %q", nodes[2].Name)
	}
}

func TestListFiles_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := newTestFileService(root)

	_, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId: "test-session",
		Path:      "../../../etc",
	}))
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestListFiles_NotFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := newTestFileService(root)

	_, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId: "test-session",
		Path:      "nonexistent",
	}))
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestListFiles_NodeCap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create more than maxDirEntries files.
	for i := 0; i <= maxDirEntries; i++ {
		name := filepath.Join(root, fmt.Sprintf("f%07d.txt", i))
		if err := os.WriteFile(name, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	svc := newTestFileService(root)
	resp, err := svc.listFiles(context.Background(), connect.NewRequest(&sessionv1.ListFilesRequest{
		SessionId: "test-session",
		Path:      ".",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.Truncated {
		t.Error("expected truncated=true when dir has >maxDirEntries entries")
	}
}

// ---- Tests for GetFileContent ----

func TestGetFileContent_TextFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.getFileContent(context.Background(), connect.NewRequest(&sessionv1.GetFileContentRequest{
		SessionId: "test-session",
		Path:      "main.go",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.IsBinary {
		t.Error("expected is_binary=false for .go file")
	}
	if resp.Msg.Content != content {
		t.Errorf("content mismatch: got %q, want %q", resp.Msg.Content, content)
	}
}

func TestGetFileContent_BinaryFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte{0x00, 0x01, 0x02, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.getFileContent(context.Background(), connect.NewRequest(&sessionv1.GetFileContentRequest{
		SessionId: "test-session",
		Path:      "data.bin",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.IsBinary {
		t.Error("expected is_binary=true for file with null bytes")
	}
	if resp.Msg.Content != "" {
		t.Error("expected empty content for binary file")
	}
}

func TestGetFileContent_FileTooLarge(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bigContent := strings.Repeat("a", maxFileSize+1)
	if err := os.WriteFile(filepath.Join(root, "huge.txt"), []byte(bigContent), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	_, err := svc.getFileContent(context.Background(), connect.NewRequest(&sessionv1.GetFileContentRequest{
		SessionId: "test-session",
		Path:      "huge.txt",
	}))
	if err == nil {
		t.Fatal("expected error for file >maxFileSize")
	}
}

func TestGetFileContent_NotFound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := newTestFileService(root)

	_, err := svc.getFileContent(context.Background(), connect.NewRequest(&sessionv1.GetFileContentRequest{
		SessionId: "test-session",
		Path:      "nonexistent.go",
	}))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestGetFileContent_PathTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := newTestFileService(root)

	_, err := svc.getFileContent(context.Background(), connect.NewRequest(&sessionv1.GetFileContentRequest{
		SessionId: "test-session",
		Path:      "../../etc/passwd",
	}))
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestGetFileContent_Truncation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content := strings.Repeat("x", truncateSize+1)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.getFileContent(context.Background(), connect.NewRequest(&sessionv1.GetFileContentRequest{
		SessionId: "test-session",
		Path:      "big.txt",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Msg.IsTruncated {
		t.Error("expected is_truncated=true for file >1MB")
	}
	if int64(len(resp.Msg.Content)) != truncateSize {
		t.Errorf("expected content length %d, got %d", truncateSize, len(resp.Msg.Content))
	}
}

// ---- Tests for SearchFiles ----

func TestSearchFiles_NestedMatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "b", "c", "target.go"), []byte("package c"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId: "test-session",
		Query:     "target",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Msg.Files) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Msg.Files))
	}
	if resp.Msg.Files[0].Name != "target.go" {
		t.Errorf("expected target.go, got %q", resp.Msg.Files[0].Name)
	}
	if resp.Msg.Files[0].Path != "a/b/c/target.go" {
		t.Errorf("expected path a/b/c/target.go, got %q", resp.Msg.Files[0].Path)
	}
}

func TestSearchFiles_HardSkipDirs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "foo.js"), []byte("module"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real_foo.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId: "test-session",
		Query:     "foo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, f := range resp.Msg.Files {
		if strings.Contains(f.Path, "node_modules") {
			t.Errorf("expected node_modules to be skipped, got %q", f.Path)
		}
	}
	found := false
	for _, f := range resp.Msg.Files {
		if f.Name == "real_foo.go" {
			found = true
		}
	}
	if !found {
		t.Error("expected real_foo.go in results")
	}
}

func TestSearchFiles_GitignoreFiltering(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.tmp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x.tmp"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)

	// Without include_ignored: x.tmp should not appear.
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId:      "test-session",
		Query:          "x.",
		IncludeIgnored: false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range resp.Msg.Files {
		if strings.HasSuffix(f.Name, ".tmp") {
			t.Errorf("expected .tmp to be filtered, got %q", f.Name)
		}
	}

	// With include_ignored: x.tmp should appear.
	resp2, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId:      "test-session",
		Query:          "x.tmp",
		IncludeIgnored: true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, f := range resp2.Msg.Files {
		if f.Name == "x.tmp" {
			found = true
		}
	}
	if !found {
		t.Error("expected x.tmp when include_ignored=true")
	}
}

func TestSearchFiles_ShortQueryReturnsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId: "test-session",
		Query:     "a",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Files) != 0 {
		t.Errorf("expected 0 results for short query, got %d", len(resp.Msg.Files))
	}
}

func TestSearchFiles_MaxResultsCap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	for i := 0; i < 10; i++ {
		name := filepath.Join(root, fmt.Sprintf("match%02d.go", i))
		if err := os.WriteFile(name, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
	}

	svc := newTestFileService(root)
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId:  "test-session",
		Query:      "match",
		MaxResults: 3,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Files) != 3 {
		t.Errorf("expected 3 results (capped), got %d", len(resp.Msg.Files))
	}
	if !resp.Msg.Truncated {
		t.Error("expected truncated=true when results capped")
	}
	if resp.Msg.TotalMatches < 10 {
		t.Errorf("expected total_matches >= 10, got %d", resp.Msg.TotalMatches)
	}
}

func TestSearchFiles_NoMatchReturnsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId: "test-session",
		Query:     "zzznomatch",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Files) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Msg.Files))
	}
	if resp.Msg.Truncated {
		t.Error("expected truncated=false for empty results")
	}
}

func TestSearchFiles_PathTraversalRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := newTestFileService(root)

	// Use unknown session ID to trigger not-found (path traversal is in session ID).
	_, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId: "../../etc",
		Query:     "passwd",
	}))
	if err == nil {
		t.Fatal("expected error for unknown session ID")
	}
}

func TestSearchFiles_PathMatchOnFullPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "src", "cmd"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "cmd", "run.go"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newTestFileService(root)
	// Query matches the path "src/cmd" but not the filename "run.go".
	resp, err := svc.searchFiles(context.Background(), connect.NewRequest(&sessionv1.SearchFilesRequest{
		SessionId: "test-session",
		Query:     "src/cmd",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Msg.Files) != 1 {
		t.Fatalf("expected 1 result for path match, got %d", len(resp.Msg.Files))
	}
	if resp.Msg.Files[0].Name != "run.go" {
		t.Errorf("expected run.go, got %q", resp.Msg.Files[0].Name)
	}
}

// ---- Tests for ServeFileRaw ----

// serveFileRawRequest is a helper that issues a GET request to ServeFileRaw for the
// given file path (relative to the test workspace) and returns the response.
func serveFileRawRequest(t *testing.T, svc *FileService, relPath string, extraQuery string) *http.Response {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(svc.ServeFileRaw))
	t.Cleanup(ts.Close)
	url := ts.URL + "?sessionId=test-session&path=" + relPath
	if extraQuery != "" {
		url += "&" + extraQuery
	}
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	return resp
}

func TestServeFileRaw_PDF(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Minimal PDF header so the file is recognisable.
	pdfContent := []byte("%PDF-1.4 minimal pdf content for testing purposes")
	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), pdfContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "doc.pdf", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %q", got)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.HasPrefix(cd, "inline") {
		t.Errorf("expected Content-Disposition to start with 'inline', got %q", cd)
	}
	if !strings.Contains(cd, "doc.pdf") {
		t.Errorf("expected filename=doc.pdf in Content-Disposition, got %q", cd)
	}
	// PDF must NOT have CSP sandbox — it would break Chrome's PDFium renderer.
	if csp := resp.Header.Get("Content-Security-Policy"); csp != "" {
		t.Errorf("expected no Content-Security-Policy for PDF, got %q", csp)
	}
}

func TestServeFileRaw_SVG(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svgContent := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`)
	if err := os.WriteFile(filepath.Join(root, "image.svg"), svgContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "image.svg", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// SVG must have CSP sandbox to prevent XSS via embedded scripts.
	if csp := resp.Header.Get("Content-Security-Policy"); csp != "sandbox" {
		t.Errorf("expected Content-Security-Policy: sandbox for SVG, got %q", csp)
	}
}

func TestServeFileRaw_HTML_RewritesRelativeLinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logos", "iterations"), 0755); err != nil {
		t.Fatal(err)
	}
	html := `<html><head></head><body>` +
		`<img src="iterations/iteration-3.svg">` +
		`<a href="/absolute.png">abs</a>` +
		`<a href="https://example.com/x.png">external</a>` +
		`</body></html>`
	if err := os.WriteFile(filepath.Join(root, "logos", "preview.html"), []byte(html), 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "logos%2Fpreview.html", "")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)

	if !strings.Contains(got, `src="/api/files/raw?path=logos%2Fiterations%2Fiteration-3.svg&sessionId=test-session"`) {
		t.Errorf("relative image src not rewritten correctly, got: %s", got)
	}
	if !strings.Contains(got, `href="/api/files/raw?path=absolute.png&sessionId=test-session"`) {
		t.Errorf("root-relative href not rewritten correctly, got: %s", got)
	}
	if !strings.Contains(got, `href="https://example.com/x.png"`) {
		t.Errorf("external href should be left untouched, got: %s", got)
	}
}

func TestServeFileRaw_VideoMP4(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Minimal ftyp box so Go's http.ServeContent doesn't choke; content-type is driven by extension.
	mp4Content := make([]byte, 512)
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), mp4Content, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "clip.mp4", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "video/mp4") {
		t.Errorf("expected Content-Type video/mp4, got %q", ct)
	}
}

func TestServeFileRaw_VideoOGV(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ogvContent := make([]byte, 512)
	if err := os.WriteFile(filepath.Join(root, "clip.ogv"), ogvContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "clip.ogv", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "video/ogg") {
		t.Errorf("expected Content-Type video/ogg for .ogv, got %q", ct)
	}
}

func TestServeFileRaw_LargeFilePDF(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// PDF larger than maxFileSize (10 MB) — must still return 200 (no size cap for PDF).
	largeContent := make([]byte, maxFileSize+1024)
	copy(largeContent, []byte("%PDF-1.4 "))
	if err := os.WriteFile(filepath.Join(root, "large.pdf"), largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "large.pdf", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for large PDF (no size cap), got %d", resp.StatusCode)
	}
}

func TestServeFileRaw_LargeFileBinary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Generic binary (null bytes) larger than maxFileSize — must return 413.
	largeContent := make([]byte, maxFileSize+1024)
	if err := os.WriteFile(filepath.Join(root, "large.bin"), largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{existingDir: root})
	resp := serveFileRawRequest(t, svc, "large.bin", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for large binary file, got %d", resp.StatusCode)
	}
}

// ---- Real-handler tests for the os.Root retrofit (Story 0.2.1) ----
//
// The testFileService helpers above re-implement the handlers, so they cannot
// catch a regression in the real FileService. These tests drive the real methods.

func realFileService(root string) *FileService {
	return NewFileService(&fakeWorkspaceProvider{existingDir: root})
}

func realGetFileContent(t *testing.T, root, rel string) (*sessionv1.GetFileContentResponse, error) {
	t.Helper()
	resp, err := realFileService(root).GetFileContent(context.Background(), connect.NewRequest(
		&sessionv1.GetFileContentRequest{SessionId: "test-session", Path: rel}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestGetFileContent_RealHandler_ShouldKeepBehavior_ForTextBinaryTruncatedAndErrors(t *testing.T) {
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, "main.go"), []byte("package main\n"))
	writeFixtureFile(t, filepath.Join(root, "data.bin"), []byte{0, 1, 2, 0})
	writeFixtureFile(t, filepath.Join(root, "nul.dat"), append([]byte("text then "), 0, 'x'))
	writeFixtureFile(t, filepath.Join(root, "big.txt"), []byte(strings.Repeat("a", truncateSize+10)))
	writeFixtureFile(t, filepath.Join(root, "huge.txt"), []byte(strings.Repeat("a", maxFileSize+1)))
	mustMkdir(t, filepath.Join(root, "dir"))

	got, err := realGetFileContent(t, root, "main.go")
	if err != nil || got.Content != "package main\n" || got.IsBinary || got.Size != 13 || got.Encoding != "utf-8" {
		t.Fatalf("text: %+v, %v", got, err)
	}
	if got, err = realGetFileContent(t, root, "data.bin"); err != nil || !got.IsBinary || got.Content != "" {
		t.Fatalf("binary: %+v, %v", got, err)
	}
	if got, err = realGetFileContent(t, root, "nul.dat"); err != nil || !got.IsBinary {
		t.Fatalf("null-byte scan: %+v, %v", got, err)
	}
	if got, err = realGetFileContent(t, root, "big.txt"); err != nil || !got.IsTruncated || len(got.Content) != truncateSize {
		t.Fatalf("truncated: truncated=%v len=%d err=%v", got.GetIsTruncated(), len(got.GetContent()), err)
	}
	for rel, want := range map[string]connect.Code{
		"huge.txt":   connect.CodeFailedPrecondition,
		"missing.go": connect.CodeNotFound,
		"dir":        connect.CodeInvalidArgument,
		"../outside": connect.CodeInvalidArgument,
		"":           connect.CodeInvalidArgument,
	} {
		if _, err = realGetFileContent(t, root, rel); connect.CodeOf(err) != want {
			t.Errorf("%q: code %v, want %v (%v)", rel, connect.CodeOf(err), want, err)
		}
	}
}

func TestGetFileContent_ShouldReturnPermissionDenied_When_LeafIsSymlinkEscape(t *testing.T) {
	dir, _ := newResolverFixture(t)
	got, err := realGetFileContent(t, dir, "leak")
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("code %v, want PermissionDenied (%v)", connect.CodeOf(err), err)
	}
	if got != nil {
		t.Fatalf("no content bytes may be returned, got %+v", got)
	}
}

// In-tree link reads carry ReadOnlyReason=Symlink; escaping links are denied with a typed
// ReadOnlyReasonDetail that carries the link target.
func TestGetFileContent_ShouldReadInTreeSymlinkReadOnly_And_DenyEscapingWithTargetDetail_When_LinkClasses(t *testing.T) {
	dir, outside := newResolverFixture(t)
	for rel, want := range map[string]string{"AGENTS.md": "claude\n", "docs/alias.md": "readme\n", "chain1": "claude\n", "shared/x.txt": "shared\n"} {
		got, err := realGetFileContent(t, dir, rel)
		if err != nil || got.Content != want {
			t.Errorf("%s: %+v, %v", rel, got, err)
			continue
		}
		if got.ReadOnlyReason != sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK {
			t.Errorf("%s: reason %v, want SYMLINK", rel, got.ReadOnlyReason)
		}
	}
	denied := map[string]string{
		"leak":            filepath.Join(outside, "secret"),
		"leak_rel":        "../" + filepath.Base(outside) + "/secret",
		"dir_leak/secret": outside,
		"gitlink":         ".git",
		"linkdir":         ".git/hooks",
	}
	for rel, target := range denied {
		got, err := realGetFileContent(t, dir, rel)
		var sym *SymlinkDeniedError
		if got != nil || connect.CodeOf(err) != connect.CodePermissionDenied || !errors.As(err, &sym) || sym.Target != target {
			t.Errorf("%s: got=%v err=%v want target %q", rel, got, err, target)
			continue
		}
		detail := readOnlyDetail(t, err)
		if detail == nil || detail.Reason != sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK || detail.LinkTarget != target {
			t.Errorf("%s: detail %+v, want SYMLINK with target %q", rel, detail, target)
		}
	}
	if _, err := realGetFileContent(t, dir, "dangling_inside"); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("dangling in-tree link: %v", err)
	}
	if _, err := realGetFileContent(t, dir, "dangling"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf("dangling link above the root: %v", err)
	}
	if _, err := realGetFileContent(t, dir, ".git/config"); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Errorf(".git read: %v", err)
	}
}

func TestServeFileRaw_ShouldRefuseEscapingSymlinkAndServeInTreeLink(t *testing.T) {
	dir, _ := newResolverFixture(t)
	svc := realFileService(dir)
	resp := serveFileRawRequest(t, svc, "leak", "")
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || strings.Contains(string(body), "TOPSECRET") {
		t.Fatalf("escape: status %d body %q", resp.StatusCode, body)
	}
	resp = serveFileRawRequest(t, svc, ".git/config", "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf(".git: status %d", resp.StatusCode)
	}
	resp = serveFileRawRequest(t, svc, "AGENTS.md", "")
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "claude\n" {
		t.Fatalf("in-tree link: status %d body %q", resp.StatusCode, body)
	}
}
func realListFiles(t *testing.T, root, rel string) (*sessionv1.ListFilesResponse, error) {
	t.Helper()
	resp, err := realFileService(root).ListFiles(context.Background(), connect.NewRequest(
		&sessionv1.ListFilesRequest{SessionId: "test-session", Path: rel, IncludeIgnored: true}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestListFiles_ShouldListInTreeDirSymlinkAndRefuseEscapingOrGitLink_When_RetrofittedToOsRoot(t *testing.T) {
	dir, _ := newResolverFixture(t)
	got, err := realListFiles(t, dir, "shared")
	if err != nil || len(got.Files) != 1 || got.Files[0].Name != "x.txt" || got.Files[0].Path != "shared/x.txt" {
		t.Fatalf("shared: %+v, %v", got, err)
	}
	for _, rel := range []string{"dir_leak", "gitlink", "linkdir", ".git", "src/.git"} {
		got, err = realListFiles(t, dir, rel)
		if got != nil || connect.CodeOf(err) != connect.CodePermissionDenied {
			t.Errorf("%s: got=%v err=%v", rel, got, err)
		}
	}
	got, err = realListFiles(t, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range got.Files {
		names[f.Name] = f.IsSymlink
	}
	if !names["AGENTS.md"] || names["src"] || names[".git"] {
		t.Errorf("root listing: %v", names)
	}
}

func TestSearchFiles_ShouldNotFollowSymlinkOutOfRoot_When_TreeSearchWalksOsRoot(t *testing.T) {
	dir, _ := newResolverFixture(t)
	resp, err := realFileService(dir).SearchFiles(context.Background(), connect.NewRequest(
		&sessionv1.SearchFilesRequest{SessionId: "test-session", Query: "secret", IncludeIgnored: true}))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range resp.Msg.Files {
		t.Errorf("unexpected match %q: the only %q file is outside the root", f.Path, "secret")
	}
	resp, err = realFileService(dir).SearchFiles(context.Background(), connect.NewRequest(
		&sessionv1.SearchFilesRequest{SessionId: "test-session", Query: "a.go", IncludeIgnored: true}))
	if err != nil || len(resp.Msg.Files) != 1 || resp.Msg.Files[0].Path != "src/a.go" {
		t.Fatalf("legit match lost: %+v, %v", resp, err)
	}
}

// readOnlyDetail extracts the typed ReadOnlyReasonDetail from a Connect error, or nil.
func readOnlyDetail(t *testing.T, err error) *sessionv1.ReadOnlyReasonDetail {
	t.Helper()
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return nil
	}
	for _, d := range ce.Details() {
		v, vErr := d.Value()
		if vErr != nil {
			continue
		}
		if det, ok := v.(*sessionv1.ReadOnlyReasonDetail); ok {
			return det
		}
	}
	return nil
}

func realStatFile(t *testing.T, root, rel string) (*sessionv1.StatFileResponse, error) {
	t.Helper()
	resp, err := realFileService(root).StatFile(context.Background(), connect.NewRequest(
		&sessionv1.StatFileRequest{SessionId: "test-session", Path: rel}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// crlfBomDoc is 120 bytes: a 3-byte BOM plus CRLF-terminated lines.
func crlfBomDoc() []byte {
	body := strings.Repeat("0123456789abcdefghi\r\n", 5) // 21 bytes x 5 = 105
	doc := append([]byte{0xEF, 0xBB, 0xBF}, []byte(body)...)
	return append(doc, []byte("0123456789\r\n")[:12]...) // 3 + 105 + 12 = 120
}

func TestStatFileAndGetFileContent_ShouldReturnTokenLineEndingBomReason_When_CrlfBomFile(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "docs"))
	doc := crlfBomDoc()
	require.Len(t, doc, 120)
	path := filepath.Join(root, "docs", "a.md")
	writeFixtureFile(t, path, doc)
	info, err := os.Stat(path)
	require.NoError(t, err)
	wantTok := NewFileVersionToken(120, info.ModTime().UnixNano(), doc).String()

	got, err := realGetFileContent(t, root, "docs/a.md")
	require.NoError(t, err)
	assert.Equal(t, wantTok, got.VersionToken)
	_, parseErr := ParseFileVersionToken(got.VersionToken)
	require.NoError(t, parseErr)
	assert.True(t, strings.HasPrefix(got.VersionToken, "v1:120:"), got.VersionToken)
	assert.Equal(t, sessionv1.LineEnding_LINE_ENDING_CRLF, got.LineEnding)
	assert.True(t, got.HasBom)
	assert.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE, got.ReadOnlyReason)
	assert.Equal(t, string(doc[3:]), got.Content, "BOM stripped, CRLF kept for the client to normalize")
	assert.Equal(t, int64(120), got.Size)

	st1, err := realStatFile(t, root, "docs/a.md")
	require.NoError(t, err)
	assert.Equal(t, got.VersionToken, st1.VersionToken)
	assert.Equal(t, int64(120), st1.Size)
	assert.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE, st1.ReadOnlyReason)
	st2, err := realStatFile(t, root, "docs/a.md")
	require.NoError(t, err)
	assert.Equal(t, st1.VersionToken, st2.VersionToken, "stable when unchanged")

	changed := append(append([]byte(nil), doc[:119]...), 'X')
	writeFixtureFile(t, path, changed)
	require.NoError(t, os.Chtimes(path, info.ModTime().Add(time.Second), info.ModTime().Add(time.Second)))
	st3, err := realStatFile(t, root, "docs/a.md")
	require.NoError(t, err)
	assert.NotEqual(t, st1.VersionToken, st3.VersionToken)
}

func TestGetFileContent_ShouldClassifyEveryReadOnlyReason_When_FixtureFiles(t *testing.T) {
	root, _ := newResolverFixture(t)
	writeFixtureFile(t, filepath.Join(root, "bin.dat"), []byte{0, 1, 2, 3})
	writeFixtureFile(t, filepath.Join(root, "latin1.txt"), []byte("caf\xE9\n"))
	writeFixtureFile(t, filepath.Join(root, "mixed.txt"), []byte("a\r\nb\nc\n"))
	writeFixtureFile(t, filepath.Join(root, "lone_cr.txt"), []byte("a\nb\rc\n"))
	writeFixtureFile(t, filepath.Join(root, "plain.txt"), []byte("a\nb\n"))
	for rel, want := range map[string]sessionv1.ReadOnlyReason{
		"bin.dat":     sessionv1.ReadOnlyReason_READ_ONLY_REASON_BINARY,
		"latin1.txt":  sessionv1.ReadOnlyReason_READ_ONLY_REASON_NON_UTF8,
		"mixed.txt":   sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL,
		"lone_cr.txt": sessionv1.ReadOnlyReason_READ_ONLY_REASON_MIXED_EOL,
		"AGENTS.md":   sessionv1.ReadOnlyReason_READ_ONLY_REASON_SYMLINK,
		"plain.txt":   sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE,
	} {
		got, err := realGetFileContent(t, root, rel)
		require.NoError(t, err, rel)
		assert.Equal(t, want, got.ReadOnlyReason, rel)
		st, err := realStatFile(t, root, rel)
		require.NoError(t, err, rel)
		assert.Equal(t, want, st.ReadOnlyReason, "StatFile "+rel)
		if want != sessionv1.ReadOnlyReason_READ_ONLY_REASON_NONE {
			assert.True(t, strings.HasSuffix(got.VersionToken, ":-"), "%s token %s carries no editable hash", rel, got.VersionToken)
		}
	}
	got, err := realGetFileContent(t, root, "latin1.txt")
	require.NoError(t, err)
	assert.True(t, utf8.ValidString(got.Content), "invalid bytes are replaced so the response can marshal")
}

func TestGetFileContent_ShouldCutOnRuneBoundaryAndCarryDashToken_When_TwoMegabyteText(t *testing.T) {
	root := t.TempDir()
	// 3-byte runes: 1 MiB is not a multiple of 3, so the raw cut lands inside a rune.
	writeFixtureFile(t, filepath.Join(root, "big.txt"), []byte(strings.Repeat("\u4e16", (2<<20)/3)))
	got, err := realGetFileContent(t, root, "big.txt")
	require.NoError(t, err)
	assert.True(t, got.IsTruncated)
	assert.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TRUNCATED, got.ReadOnlyReason)
	assert.True(t, strings.HasSuffix(got.VersionToken, ":-"), got.VersionToken)
	assert.True(t, utf8.ValidString(got.Content))
	assert.LessOrEqual(t, len(got.Content), truncateSize)
	assert.Greater(t, len(got.Content), truncateSize-utf8.UTFMax)
	assert.Equal(t, int64((2<<20)/3*3), got.Size)

	st, err := realStatFile(t, root, "big.txt")
	require.NoError(t, err)
	assert.Equal(t, got.VersionToken, st.VersionToken)
}

func TestStatFile_ShouldReportNotFoundDirAndTooLarge_Without_Content(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "dir"))
	writeFixtureFile(t, filepath.Join(root, "huge.txt"), []byte(strings.Repeat("a", maxFileSize+1)))
	_, err := realStatFile(t, root, "missing.go")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = realStatFile(t, root, "dir")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = realStatFile(t, root, "../x")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	st, err := realStatFile(t, root, "huge.txt")
	require.NoError(t, err)
	assert.Equal(t, sessionv1.ReadOnlyReason_READ_ONLY_REASON_TOO_LARGE, st.ReadOnlyReason)
	assert.Equal(t, int64(maxFileSize+1), st.Size)
	_, err = realFileService(root).StatFile(context.Background(), connect.NewRequest(&sessionv1.StatFileRequest{}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
