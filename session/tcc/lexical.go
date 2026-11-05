package tcc

import (
	"path/filepath"
	"strings"
)

// LexicalPolicy does no filesystem I/O. Unknown locations require readiness
// approval too; it cannot identify mounted filesystem types without probing.
func LexicalPolicy(home string) func(string) Result {
	home = lexicalDarwinPath(home)
	safe := []string{filepath.Join(home, "code"), filepath.Join(home, ".stapler-squad")}
	return func(path string) Result {
		path = lexicalDarwinPath(path)
		for _, root := range safe {
			if within(path, root) || within(root, path) {
				return Result{}
			}
		}
		for _, name := range []string{"Desktop", "Documents", "Downloads", "Library"} {
			root := filepath.Join(home, name)
			if within(path, root) {
				return Result{Protected: true, Root: root, Service: FilesAndFolders}
			}
		}
		if within(path, home) {
			rel := strings.TrimPrefix(path, home+string(filepath.Separator))
			first := splitFirst(rel)
			return Result{Protected: true, Root: filepath.Join(home, first), Service: FilesAndFolders}
		}
		if path == "/" || path == "/Users" || path == "/private" || path == "/private/var" || path == "/private/var/folders" {
			return Result{}
		}
		root := filepath.Join("/", splitFirst(path))
		return Result{Protected: true, Root: root, Service: FilesAndFolders}
	}
}

func lexicalDarwinPath(path string) string {
	path = filepath.Clean(path)
	if path == "/var" || strings.HasPrefix(path, "/var/") {
		return "/private" + path
	}
	if path == "/tmp" || strings.HasPrefix(path, "/tmp/") {
		return "/private" + path
	}
	return path
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !startsParent(rel)
}
func startsParent(rel string) bool { return len(rel) > 3 && rel[:3] == "../" }
func splitFirst(path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for i, c := range path {
		if c == '/' {
			return path[:i]
		}
	}
	return path
}
