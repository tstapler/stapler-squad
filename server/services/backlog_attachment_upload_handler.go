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
)

const (
	backlogAttachmentMaxBytes = 10 * 1024 * 1024 // 10 MB
	backlogAttachmentDirMode  = 0o750
	backlogAttachmentFileMode = 0o600
)

// backlogAttachmentAllowedTypes maps sniffed content types (via http.DetectContentType,
// not the client-declared MIME type or filename extension) to a safe file extension.
// Deliberately excludes image/svg+xml — SVG is XML and can embed <script>.
var backlogAttachmentAllowedTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// BacklogAttachmentUploadHandler saves uploaded images for backlog item
// descriptions to a durable directory, independent of any session.
//
// ponytail: uploads aren't tracked by item ID, so a file can outlive its
// backlog item (upload succeeds, item creation fails/is deleted) and become
// an orphan on disk. Accepted as YAGNI until proven — add a tracked
// attachment list + delete-on-item-delete wiring if orphan growth becomes a
// real problem.
type BacklogAttachmentUploadHandler struct {
	dir string
}

// NewBacklogAttachmentUploadHandler creates a handler that saves into dir,
// creating it if necessary.
func NewBacklogAttachmentUploadHandler(dir string) *BacklogAttachmentUploadHandler {
	if err := os.MkdirAll(dir, backlogAttachmentDirMode); err != nil {
		log.Error("[BacklogAttachmentUpload] cannot create dir", "dir", dir, "err", err)
	}
	return &BacklogAttachmentUploadHandler{dir: dir}
}

type backlogAttachmentUploadResponse struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
}

// +http: POST /api/v1/upload-backlog-attachment upload:backlog-attachment
// HandleUpload processes a multipart/form-data POST with a "file" field.
// Unlike the session upload handler, this validates the file is a real
// raster image via magic bytes — not just the declared MIME type or
// filename extension — since these attachments are embedded directly into
// markdown and rendered without further review.
func (h *BacklogAttachmentUploadHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, backlogAttachmentMaxBytes+64*1024)
	if err := r.ParseMultipartForm(backlogAttachmentMaxBytes); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			http.Error(w, "file too large (max 10 MB)", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	buf = buf[:n]
	if len(buf) == 0 {
		http.Error(w, "uploaded file is empty", http.StatusBadRequest)
		return
	}

	sniffed := http.DetectContentType(buf)
	ext, ok := backlogAttachmentAllowedTypes[sniffed]
	if !ok {
		http.Error(w, fmt.Sprintf("unsupported file type: %s (only PNG, JPEG, GIF, and WebP images are allowed)", sniffed), http.StatusUnsupportedMediaType)
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Sanitize the client-supplied filename, then force the extension to match
	// the sniffed content type — the declared filename extension is untrusted.
	safeName := sanitizeFilename(header.Filename)
	safeStem := strings.TrimSuffix(safeName, filepath.Ext(safeName))
	ts := time.Now().UnixMilli()
	pattern := fmt.Sprintf("%d-*-%s%s", ts, safeStem, ext)

	f, err := os.CreateTemp(h.dir, pattern)
	if err != nil {
		log.Error("[BacklogAttachmentUpload] CreateTemp failed", "dir", h.dir, "err", err)
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	savedPath := f.Name()

	var writeErr error
	defer func() {
		if writeErr != nil {
			os.Remove(savedPath) //nolint:errcheck
		}
	}()

	if _, writeErr = io.Copy(f, file); writeErr != nil {
		f.Close()
		log.Error("[BacklogAttachmentUpload] write failed", "err", writeErr)
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}
	f.Close()

	if err := os.Chmod(savedPath, backlogAttachmentFileMode); err != nil {
		log.Error("[BacklogAttachmentUpload] chmod failed (non-fatal)", "err", err)
	}

	log.Info("[BacklogAttachmentUpload] saved file", "path", savedPath)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(backlogAttachmentUploadResponse{
		Path:     savedPath,
		Filename: filepath.Base(savedPath),
	})
}
