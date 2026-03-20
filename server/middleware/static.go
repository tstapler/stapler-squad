package middleware

import (
	"github.com/tstapler/stapler-squad/log"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// StaticFileServer creates an HTTP handler that serves static files from an embedded filesystem.
// It implements SPA (Single Page Application) routing by serving index.html for non-file routes.
func StaticFileServer(fileSystem fs.FS, indexFile string) http.Handler {
	// Create file server with the embedded filesystem
	fileServer := http.FileServer(http.FS(fileSystem))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to prevent directory traversal
		cleanPath := path.Clean(r.URL.Path)

		// Remove leading slash for fs.Stat
		fsPath := strings.TrimPrefix(cleanPath, "/")
		if fsPath == "" {
			fsPath = "."
		}

		// Check if the file exists in the filesystem
		fileInfo, err := fs.Stat(fileSystem, fsPath)

		if err != nil {
			// File doesn't exist - serve root index.html for SPA routing
			log.InfoLog.Printf("Serving %s for SPA route: %s (file not found)", indexFile, cleanPath)

			// Read and serve the root index file directly
			indexContent, err := fs.ReadFile(fileSystem, indexFile)
			if err != nil {
				http.Error(w, "Index file not found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.WriteHeader(http.StatusOK)
			w.Write(indexContent)
			return
		}

		if fileInfo.IsDir() {
			// It's a directory - try to serve index.html from within that directory
			dirIndexPath := path.Join(fsPath, "index.html")
			dirIndexContent, err := fs.ReadFile(fileSystem, dirIndexPath)
			if err == nil {
				// Found index.html in the directory - serve it
				log.InfoLog.Printf("Serving directory index: %s", dirIndexPath)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				w.WriteHeader(http.StatusOK)
				w.Write(dirIndexContent)
				return
			}

			// No index.html in directory - serve root index.html for SPA routing
			log.InfoLog.Printf("Serving %s for SPA route: %s (directory without index)", indexFile, cleanPath)
			rootIndexContent, err := fs.ReadFile(fileSystem, indexFile)
			if err != nil {
				http.Error(w, "Index file not found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.WriteHeader(http.StatusOK)
			w.Write(rootIndexContent)
			return
		}

		// Set cache headers for static assets
		if strings.HasPrefix(cleanPath, "/_next/") {
			// Next.js assets are immutable and can be cached indefinitely
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(cleanPath, ".html") {
			// HTML files should not be cached (for SPA routing)
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}

		// Serve the file
		fileServer.ServeHTTP(w, r)
	})
}
