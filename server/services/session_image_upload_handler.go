package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const (
	sessionUploadMaxBytes = 10 * 1024 * 1024 // 10 MB
	sessionUploadDirMode  = 0o750
	sessionUploadFileMode = 0o600
)

// SessionImageUploadHandler saves uploaded images to a session's uploads/ directory
// and returns the absolute path so the terminal process can reference the file.
type SessionImageUploadHandler struct {
	storage session.InstanceStore
}

// NewSessionImageUploadHandler creates a new handler backed by the given InstanceStore.
func NewSessionImageUploadHandler(storage session.InstanceStore) *SessionImageUploadHandler {
	return &SessionImageUploadHandler{storage: storage}
}

// sessionImageUploadResponse is the JSON returned on success.
type sessionImageUploadResponse struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
}

// isAllowedImageType strips parameters from the content-type and checks the allowlist.
func isAllowedImageType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	switch ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// detectWebP returns true when buf contains the RIFF....WEBP container magic bytes.
// http.DetectContentType does not recognise WebP, so this is a manual check.
func detectWebP(buf []byte) bool {
	return len(buf) >= 12 &&
		buf[0] == 'R' && buf[1] == 'I' && buf[2] == 'F' && buf[3] == 'F' &&
		buf[8] == 'W' && buf[9] == 'E' && buf[10] == 'B' && buf[11] == 'P'
}

// sanitizeFilename strips path components and limits length to 100 characters.
// Returns "upload" if the sanitized result is empty or a reserved name.
func sanitizeFilename(name string) string {
	// filepath.Base removes any directory component (path traversal defense).
	name = filepath.Base(name)
	// Replace any remaining path separators and null bytes.
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, name)
	// Limit to 100 characters preserving the extension when feasible.
	if len(name) > 100 {
		ext := filepath.Ext(name)
		prefixLen := 100 - len(ext)
		if prefixLen > 0 {
			name = name[:prefixLen] + ext
		} else {
			// Extension itself is too long — truncate the whole thing.
			name = name[:100]
		}
	}
	if name == "" || name == "." || name == ".." {
		name = "upload"
	}
	return name
}

// +http: POST /api/v1/upload-image upload:image
// HandleUpload processes a multipart/form-data POST with fields "session_id" and "file",
// saves the image to <session_path>/uploads/ and returns the absolute path as JSON.
func (h *SessionImageUploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit body to 10 MB file + multipart overhead (boundaries, headers, session_id field).
	r.Body = http.MaxBytesReader(w, r.Body, sessionUploadMaxBytes+64*1024)
	if err := r.ParseMultipartForm(sessionUploadMaxBytes); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "file too large (max 10 MB)", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad multipart form", http.StatusBadRequest)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read first 512 bytes for MIME sniffing.
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	buf = buf[:n]

	if len(buf) == 0 {
		http.Error(w, "uploaded file is empty", http.StatusBadRequest)
		return
	}

	var detectedType string
	if detectWebP(buf) {
		detectedType = "image/webp"
	} else {
		detectedType = http.DetectContentType(buf)
	}

	if !isAllowedImageType(detectedType) {
		http.Error(w, fmt.Sprintf("unsupported image type %q (allowed: jpeg, png, gif, webp)", detectedType), http.StatusBadRequest)
		return
	}

	// Seek back so the full file (including the first 512 bytes) is written to disk.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Resolve session path via InstanceStore.
	instances, err := h.storage.LoadInstances()
	if err != nil {
		log.ErrorLog.Printf("[SessionImageUpload] LoadInstances: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var inst *session.Instance
	for _, i := range instances {
		if i.ID == sessionID {
			inst = i
			break
		}
	}
	if inst == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if inst.Path == "" {
		http.Error(w, "session has no working directory", http.StatusUnprocessableEntity)
		return
	}

	// Verify the session's working directory exists on disk before attempting
	// to create the uploads/ subdirectory — paused sessions may have had their
	// worktree removed.
	if _, statErr := os.Stat(inst.Path); statErr != nil {
		http.Error(w, "session working directory is not accessible", http.StatusUnprocessableEntity)
		return
	}

	uploadsDir := filepath.Join(inst.Path, "uploads")
	if err := os.MkdirAll(uploadsDir, sessionUploadDirMode); err != nil {
		log.ErrorLog.Printf("[SessionImageUpload] MkdirAll %s: %v", uploadsDir, err)
		http.Error(w, "failed to create uploads directory", http.StatusInternalServerError)
		return
	}

	safeName := sanitizeFilename(header.Filename)
	ts := time.Now().UnixMilli()
	pattern := fmt.Sprintf("%d-*-%s", ts, safeName)

	f, err := os.CreateTemp(uploadsDir, pattern)
	if err != nil {
		log.ErrorLog.Printf("[SessionImageUpload] CreateTemp in %s: %v", uploadsDir, err)
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	savedPath := f.Name()

	// Deferred cleanup on write error — avoids partial files on disk.
	var writeErr error
	defer func() {
		if writeErr != nil {
			os.Remove(savedPath) //nolint:errcheck
		}
	}()

	if _, writeErr = io.Copy(f, file); writeErr != nil {
		f.Close()
		log.ErrorLog.Printf("[SessionImageUpload] write failed: %v", writeErr)
		http.Error(w, "failed to save image", http.StatusInternalServerError)
		return
	}
	f.Close()

	if err := os.Chmod(savedPath, sessionUploadFileMode); err != nil {
		log.ErrorLog.Printf("[SessionImageUpload] chmod failed (non-fatal): %v", err)
	}

	log.InfoLog.Printf("[SessionImageUpload] session=%s saved → %s", sessionID, savedPath)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sessionImageUploadResponse{
		Path:     savedPath,
		Filename: filepath.Base(savedPath),
	})
}
