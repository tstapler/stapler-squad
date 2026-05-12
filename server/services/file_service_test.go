package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"

	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// fakeWorkspaceProvider implements WorkspaceProvider for tests.
type fakeWorkspaceProvider struct {
	effectivePath string
}

func (f *fakeWorkspaceProvider) GetWorkspace(sessionID string) (session.Workspace, error) {
	if sessionID != "test-session" {
		return session.Workspace{}, connect.NewError(connect.CodeNotFound, nil)
	}
	return session.Workspace{EffectivePath: f.effectivePath}, nil
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
			_, err := resolveAndValidatePath(base, tc.rel)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.rel)
			}
		})
	}
}

func TestResolveAndValidatePath_ValidPath(t *testing.T) {
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
	root := t.TempDir()
	// Minimal PDF header so the file is recognisable.
	pdfContent := []byte("%PDF-1.4 minimal pdf content for testing purposes")
	if err := os.WriteFile(filepath.Join(root, "doc.pdf"), pdfContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{effectivePath: root})
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
	root := t.TempDir()
	svgContent := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`)
	if err := os.WriteFile(filepath.Join(root, "image.svg"), svgContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{effectivePath: root})
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

func TestServeFileRaw_VideoMP4(t *testing.T) {
	root := t.TempDir()
	// Minimal ftyp box so Go's http.ServeContent doesn't choke; content-type is driven by extension.
	mp4Content := make([]byte, 512)
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), mp4Content, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{effectivePath: root})
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
	root := t.TempDir()
	ogvContent := make([]byte, 512)
	if err := os.WriteFile(filepath.Join(root, "clip.ogv"), ogvContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{effectivePath: root})
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
	root := t.TempDir()
	// PDF larger than maxFileSize (10 MB) — must still return 200 (no size cap for PDF).
	largeContent := make([]byte, maxFileSize+1024)
	copy(largeContent, []byte("%PDF-1.4 "))
	if err := os.WriteFile(filepath.Join(root, "large.pdf"), largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{effectivePath: root})
	resp := serveFileRawRequest(t, svc, "large.pdf", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for large PDF (no size cap), got %d", resp.StatusCode)
	}
}

func TestServeFileRaw_LargeFileBinary(t *testing.T) {
	root := t.TempDir()
	// Generic binary (null bytes) larger than maxFileSize — must return 413.
	largeContent := make([]byte, maxFileSize+1024)
	if err := os.WriteFile(filepath.Join(root, "large.bin"), largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	svc := NewFileService(&fakeWorkspaceProvider{effectivePath: root})
	resp := serveFileRawRequest(t, svc, "large.bin", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for large binary file, got %d", resp.StatusCode)
	}
}
