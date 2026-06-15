package services

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// LocalFileService serves arbitrary local filesystem paths over HTTP.
// Authentication is handled by the server middleware chain — local HTTP has
// no auth, remote HTTPS requires WebAuthn.
type LocalFileService struct{}

// NewLocalFileService creates a LocalFileService.
func NewLocalFileService() *LocalFileService {
	return &LocalFileService{}
}

type localFileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type listDirectoryResponse struct {
	Path    string           `json:"path"`
	Parent  string           `json:"parent"`
	Entries []localFileEntry `json:"entries"`
	Total   int              `json:"total"`
	HasMore bool             `json:"has_more"`
}

const maxLocalDirEntries = 2000

// Auth is deliberately delegated to the server middleware chain: local HTTP callers
// already have OS access; remote HTTPS callers are gated by WebAuthn. No path
// restriction is applied — this is intentional for a personal developer tool.

// ListLocalDirectory handles GET /api/local/files/list?path=/some/dir
// Defaults to the user home directory when path is omitted.
func (s *LocalFileService) ListLocalDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawPath := r.URL.Query().Get("path")
	if rawPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			http.Error(w, "could not determine home directory", http.StatusInternalServerError)
			return
		}
		rawPath = home
	}

	dir := filepath.Clean(rawPath)
	if !filepath.IsAbs(dir) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "directory not found", http.StatusNotFound)
			return
		}
		if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		log.Error("[LocalFileService] ReadDir failed", "path", dir, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	entries := make([]localFileEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		entries = append(entries, localFileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(dir, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	// Directories first, then files, both sorted alphabetically.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	total := len(entries)
	hasMore := false
	if total > maxLocalDirEntries {
		entries = entries[:maxLocalDirEntries]
		hasMore = true
	}

	parent := ""
	if dir != "/" {
		parent = filepath.Dir(dir)
	}

	log.Info("[LocalFileService] list", "path", dir, "entries", len(entries), "total", total, "has_more", hasMore)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(listDirectoryResponse{
		Path:    dir,
		Parent:  parent,
		Entries: entries,
		Total:   total,
		HasMore: hasMore,
	}); err != nil {
		log.Error("[LocalFileService] encode response", "err", err)
	}
}

// ServeLocalFile handles GET /api/local/serve/<absolute-path>.
// The path arrives after StripPrefix removes "/api/local/serve", leaving
// the absolute filesystem path (double leading slash is normalised by
// filepath.Clean on Unix).
func (s *LocalFileService) ServeLocalFile(w http.ResponseWriter, r *http.Request) {
	filePath := filepath.Clean(r.URL.Path)
	if !filepath.IsAbs(filePath) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		http.Error(w, "path is a directory; use /api/local/files/list", http.StatusBadRequest)
		return
	}

	log.Info("[LocalFileService] serve", "path", filePath)
	http.ServeFile(w, r, filePath)
}
