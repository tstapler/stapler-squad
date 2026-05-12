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
	maxImageBytes   = 20 * 1024 * 1024 // 20 MB
	imageFileMode   = 0o600            // owner read/write only — clipboard images are private
	pasteDirMode    = 0o700            // owner access only
	maxPasteFileAge = 24 * time.Hour   // evict files older than this at startup
)

// ImageUploadHandler saves clipboard images to a temp directory and returns
// the absolute path so the terminal process can reference the file.
type ImageUploadHandler struct {
	dir string
}

func NewImageUploadHandler(dir string) *ImageUploadHandler {
	if err := os.MkdirAll(dir, pasteDirMode); err != nil {
		log.Error("[ImageUpload] cannot create paste dir", "dir", dir, "err", err)
	}
	cleanOldPasteFiles(dir)
	return &ImageUploadHandler{dir: dir}
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
			log.Info("[ImageUpload] evicted old paste file", "path", path)
		}
	}
}

type imageUploadRequest struct {
	Data        string `json:"data"`        // base64-encoded image bytes (no data-URL prefix)
	ContentType string `json:"contentType"` // e.g. "image/png"
}

type imageUploadResponse struct {
	Path string `json:"path"`
}

func (h *ImageUploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImageBytes)

	var req imageUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Data == "" {
		http.Error(w, "data is required", http.StatusBadRequest)
		return
	}
	ext := extensionFor(req.ContentType)
	if ext == "" {
		http.Error(w, "unsupported content type", http.StatusBadRequest)
		return
	}

	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		http.Error(w, "invalid base64 data", http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "image data is empty", http.StatusBadRequest)
		return
	}

	// Use os.CreateTemp so the kernel guarantees a unique filename (no collision risk).
	f, err := os.CreateTemp(h.dir, "paste-*"+ext)
	if err != nil {
		log.Error("[ImageUpload] create temp file failed", "err", err)
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	path := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		log.Error("[ImageUpload] write failed", "err", err)
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	f.Close()

	if err := os.Chmod(path, imageFileMode); err != nil {
		log.Error("[ImageUpload] chmod failed", "err", err)
	}

	log.Info("[ImageUpload] saved image", "bytes", len(data), "path", path)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(imageUploadResponse{Path: path})
}

// extensionFor maps an image content-type to a file extension.
// Returns "" for unrecognised types so the caller can reject them.
func extensionFor(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
