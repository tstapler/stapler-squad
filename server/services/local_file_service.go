package services

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxLocalDirEntries = 5000

// LocalFileEntry is one item in a directory listing.
type LocalFileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

type localDirListing struct {
	Path      string           `json:"path"`
	Entries   []LocalFileEntry `json:"entries"`
	Truncated bool             `json:"truncated,omitempty"`
}

// LocalFileService serves local filesystem files without requiring a session.
// Access control is delegated entirely to the server's auth middleware:
// local HTTP has no auth; remote HTTPS requires WebAuthn.
type LocalFileService struct{}

// NewLocalFileService creates a LocalFileService.
func NewLocalFileService() *LocalFileService {
	return &LocalFileService{}
}

// ListLocalDirectory returns a JSON directory listing.
// GET /api/local/files/list?path=/absolute/path
func (s *LocalFileService) ListLocalDirectory(w http.ResponseWriter, r *http.Request) {
	absPath := r.URL.Query().Get("path")
	if absPath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	if !filepath.IsAbs(absPath) {
		http.Error(w, "path must be absolute", http.StatusBadRequest)
		return
	}
	absPath = filepath.Clean(absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "path not found", http.StatusNotFound)
		} else if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
		} else {
			http.Error(w, "could not stat path", http.StatusInternalServerError)
		}
		return
	}
	if !info.IsDir() {
		http.Error(w, "path is not a directory", http.StatusBadRequest)
		return
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
		} else {
			http.Error(w, "could not read directory", http.StatusInternalServerError)
		}
		return
	}

	truncated := len(dirEntries) > maxLocalDirEntries
	if truncated {
		dirEntries = dirEntries[:maxLocalDirEntries]
	}

	entries := make([]LocalFileEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		fi, fiErr := de.Info()
		if fiErr != nil {
			continue
		}
		entries = append(entries, LocalFileEntry{
			Name:  de.Name(),
			Path:  filepath.Join(absPath, de.Name()),
			IsDir: de.IsDir(),
			Size:  fi.Size(),
		})
	}

	// Directories first, then files — both sorted case-insensitively.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	listing := localDirListing{Path: absPath, Entries: entries, Truncated: truncated}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(listing)
}

// ServeLocalFile serves a local file by its absolute path extracted from the URL.
// Mounted at /api/local/serve/ (with StripPrefix) so the path portion of the URL
// is the absolute filesystem path, allowing HTML relative asset references to
// resolve as sibling requests to the same prefix.
//
// No root restriction is enforced — this service is designed for local filesystem
// browsing where the user has the same access as the running process. On HTTPS
// remote access, the WebAuthn middleware is the sole gate; on localhost no auth
// is required by design (same model as opening a file manager).
//
// Security headers:
//   - SVG: Content-Security-Policy: sandbox (prevents embedded script execution)
//   - HTML: CSP sandbox allowing scripts/forms/popups but blocking parent frame access
//   - All: X-Frame-Options: SAMEORIGIN (only our own pages may embed these)
//   - All: X-Content-Type-Options: nosniff (prevents MIME-type confusion attacks)
func (s *LocalFileService) ServeLocalFile(w http.ResponseWriter, r *http.Request) {
	// After StripPrefix the URL path is already the absolute filesystem path.
	absPath := filepath.Clean(r.URL.Path)
	if !filepath.IsAbs(absPath) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
		} else if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
		} else {
			http.Error(w, "could not stat file", http.StatusInternalServerError)
		}
		return
	}
	if info.IsDir() {
		http.Error(w, "path is a directory; use /api/local/files/list", http.StatusBadRequest)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		if os.IsPermission(err) {
			http.Error(w, "permission denied", http.StatusForbidden)
		} else {
			http.Error(w, "could not open file", http.StatusInternalServerError)
		}
		return
	}
	defer func() { _ = f.Close() }()

	ext := strings.ToLower(filepath.Ext(absPath))

	// Content-type: explicit video override → extension lookup → sniff.
	contentType := videoMIMEOverrides[ext]
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "" {
		buf := make([]byte, 512)
		n, _ := f.Read(buf)
		contentType = http.DetectContentType(buf[:n])
		if _, seekErr := f.Seek(0, 0); seekErr != nil {
			http.Error(w, "could not seek file", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	lct := strings.ToLower(contentType)
	switch {
	case strings.Contains(lct, "svg"):
		w.Header().Set("Content-Security-Policy", "sandbox")
	case strings.Contains(lct, "html"):
		// Permit scripts so the page renders, but block access to the parent frame.
		w.Header().Set("Content-Security-Policy",
			"sandbox allow-scripts allow-forms allow-popups allow-modals allow-downloads")
	}

	http.ServeContent(w, r, filepath.Base(absPath), info.ModTime(), f)
}
