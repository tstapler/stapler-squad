package services

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

const (
	maxUploadBytes  = 20 * 1024 * 1024 // 20 MB (decoded file bytes)
	uploadFileMode  = 0o600            // owner read/write only — clipboard files are private
	pasteDirMode    = 0o700            // owner access only
	maxPasteFileAge = 24 * time.Hour   // evict files older than this at startup
)

// FileUploadHandler saves uploaded files to a temp directory and returns
// the absolute path so the terminal process can reference the file.
type FileUploadHandler struct {
	dir string
}

func NewFileUploadHandler(dir string) *FileUploadHandler {
	if err := os.MkdirAll(dir, pasteDirMode); err != nil {
		log.Error("[FileUpload] cannot create paste dir", "dir", dir, "err", err)
	}
	cleanOldPasteFiles(dir)
	return &FileUploadHandler{dir: dir}
}

// cleanOldPasteFiles removes paste files older than maxPasteFileAge to prevent tmpfs bloat.
func cleanOldPasteFiles(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxPasteFileAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if removeErr := os.Remove(path); removeErr == nil {
			log.Info("[FileUpload] evicted old paste file", "path", path)
		}
	}
}

type fileUploadRequest struct {
	Data             string `json:"data"`             // base64-encoded file bytes (no data-URL prefix)
	ContentType      string `json:"contentType"`      // browser-reported MIME type
	OriginalFilename string `json:"originalFilename"` // optional; used only for ext fallback
}

type fileUploadResponse struct {
	Path string `json:"path"`
}

func (h *FileUploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Base64 encoding inflates by 4/3; add 4096 bytes for JSON overhead.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes*4/3+4096)

	var req fileUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Data == "" {
		http.Error(w, "data is required", http.StatusBadRequest)
		return
	}

	safeOrigExt := sanitizeExtension(req.OriginalFilename)
	ext := extensionForMIME(req.ContentType, safeOrigExt)

	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		http.Error(w, "invalid base64 data", http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "file data is empty", http.StatusBadRequest)
		return
	}
	if len(data) > maxUploadBytes {
		http.Error(w, "file exceeds 20 MB limit", http.StatusRequestEntityTooLarge)
		return
	}

	// Use os.CreateTemp so the kernel guarantees a unique filename (no collision risk).
	f, err := os.CreateTemp(h.dir, "paste-*"+ext)
	if err != nil {
		log.Error("[FileUpload] create temp file failed", "err", err)
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	path := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		log.Error("[FileUpload] write failed", "err", err)
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	f.Close()

	if err := os.Chmod(path, uploadFileMode); err != nil {
		log.Error("[FileUpload] chmod failed", "err", err)
	}

	log.Info("[FileUpload] saved file", "bytes", len(data), "path", path)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(fileUploadResponse{Path: path})
}

// mimeExtensions is the priority table for MIME type → file extension.
// Keys must be lowercase with no parameters. Values include the leading dot.
// Covers the most common types encountered in developer workflows (40+ entries).
var mimeExtensions = map[string]string{
	// Images
	"image/png":    ".png",
	"image/jpeg":   ".jpg",
	"image/jpg":    ".jpg",
	"image/gif":    ".gif",
	"image/webp":   ".webp",
	"image/svg+xml": ".svg",
	"image/bmp":    ".bmp",
	"image/tiff":   ".tiff",
	"image/ico":    ".ico",
	"image/x-icon": ".ico",
	// Text / code
	"text/plain":      ".txt",
	"text/html":       ".html",
	"text/css":        ".css",
	"text/javascript": ".js",
	"text/typescript": ".ts",
	"text/x-python":   ".py",
	"text/x-go":       ".go",
	"text/x-rust":     ".rs",
	"text/x-c":        ".c",
	"text/x-c++":      ".cpp",
	"text/x-java":     ".java",
	"text/x-ruby":     ".rb",
	"text/x-sh":       ".sh",
	"text/xml":        ".xml",
	"text/csv":        ".csv",
	"text/markdown":   ".md",
	// Application / code
	"application/json":                                                   ".json",
	"application/javascript":                                             ".js",
	"application/typescript":                                             ".ts",
	"application/xml":                                                    ".xml",
	"application/yaml":                                                   ".yaml",
	"application/x-yaml":                                                 ".yaml",
	"application/toml":                                                   ".toml",
	"application/pdf":                                                    ".pdf",
	"application/msword":                                                 ".doc",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
	"application/vnd.ms-excel":                                           ".xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
	// Archives
	"application/zip":              ".zip",
	"application/gzip":             ".gz",
	"application/x-gzip":           ".gz",
	"application/x-tar":            ".tar",
	"application/x-bzip2":          ".bz2",
	"application/x-7z-compressed":  ".7z",
	"application/x-rar-compressed": ".rar",
	// Generic
	"application/octet-stream": ".bin",
}

// extensionForMIME returns a safe file extension for the given MIME type.
// Priority: (1) mimeExtensions table, (2) sanitized original filename extension,
// (3) ".bin" fallback.
//
// contentType may contain parameters ("text/plain; charset=utf-8") — they are stripped.
// originalExt is the extension from the client-supplied filename (e.g. ".go"), already
// sanitized by sanitizeExtension before being passed here.
func extensionForMIME(contentType, originalExt string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if ext, ok := mimeExtensions[ct]; ok {
		return ext
	}
	if originalExt != "" {
		return originalExt
	}
	return ".bin"
}

// sanitizeExtension returns a safe file extension from a client-supplied filename.
// It extracts the extension with filepath.Ext, then strips all characters that are
// not ASCII alphanumeric or '.'. If the result is empty or longer than 10 characters,
// it returns "". The returned string includes the leading dot (e.g. ".go").
//
// This prevents path traversal via crafted extensions such as "../../etc/passwd".
func sanitizeExtension(filename string) string {
	ext := filepath.Ext(filepath.Base(filename)) // strips directory components first
	if ext == "" {
		return ""
	}
	// Allow only [a-zA-Z0-9.] — strip everything else
	var b strings.Builder
	for _, r := range ext {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' {
			b.WriteRune(r)
		}
	}
	safe := strings.ToLower(b.String())
	if len(safe) > 10 || safe == "." {
		return ""
	}
	return safe
}
