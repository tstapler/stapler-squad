package middleware

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	kgzip "github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

var (
	zstdPool = sync.Pool{
		New: func() any {
			enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
			return enc
		},
	}
	gzipPool = sync.Pool{
		New: func() any {
			gz, _ := kgzip.NewWriterLevel(io.Discard, kgzip.DefaultCompression)
			return gz
		},
	}
)

// compressResponseWriter wraps http.ResponseWriter to transparently compress the response.
// Flush() flushes the compression buffer before flushing the underlying connection so
// streaming responses (ConnectRPC server-streaming) reach the client in real time.
type compressResponseWriter struct {
	http.ResponseWriter
	cw           io.WriteCloser             // gzip or zstd encoder; nil until first write
	flusher      interface{ Flush() error } // zstd.Encoder; nil for gzip (uses cw as Flusher)
	encoding     string                     // "gzip" or "zstd" — sent in Content-Encoding header
	wroteHeader  bool
	skipCompress bool
	pool         *sync.Pool // non-nil when cw was borrowed from a pool
}

func (c *compressResponseWriter) Header() http.Header {
	return c.ResponseWriter.Header()
}

func (c *compressResponseWriter) WriteHeader(code int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true

	// Skip compression for Server-Sent Events
	if strings.HasPrefix(c.ResponseWriter.Header().Get("Content-Type"), "text/event-stream") {
		c.skipCompress = true
		c.ResponseWriter.WriteHeader(code)
		return
	}

	c.ResponseWriter.Header().Set("Content-Encoding", c.encoding)
	c.ResponseWriter.Header().Del("Content-Length") // compressed length differs

	switch c.encoding {
	case "zstd":
		enc := zstdPool.Get().(*zstd.Encoder)
		enc.Reset(c.ResponseWriter)
		c.cw = enc
		c.flusher = enc
		c.pool = &zstdPool
	default: // "gzip"
		gz := gzipPool.Get().(*kgzip.Writer)
		gz.Reset(c.ResponseWriter)
		c.cw = gz
		c.pool = &gzipPool
	}

	c.ResponseWriter.WriteHeader(code)
}

func (c *compressResponseWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	if c.skipCompress || c.cw == nil {
		return c.ResponseWriter.Write(b)
	}
	return c.cw.Write(b)
}

// Flush flushes the compression buffer then the underlying connection.
// Critical for streaming: without this, compressed frames stay buffered
// until the encoder accumulates enough data to emit a block.
func (c *compressResponseWriter) Flush() {
	if c.flusher != nil {
		// zstd encoder has a typed Flush() method
		_ = c.flusher.Flush()
	} else if gz, ok := c.cw.(*kgzip.Writer); ok {
		_ = gz.Flush()
	}
	if flusher, ok := c.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack passes WebSocket upgrade requests through to the underlying connection.
func (c *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := c.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (c *compressResponseWriter) close() {
	if c.cw != nil {
		_ = c.cw.Close()
		if c.pool != nil {
			c.pool.Put(c.cw)
			c.pool = nil
		}
		c.cw = nil
	}
}

// negotiateEncoding returns the best compression encoding the client accepts.
// Prefers zstd (better ratio + speed) over gzip, returns "" if neither accepted.
func negotiateEncoding(acceptEncoding string) string {
	if strings.Contains(acceptEncoding, "zstd") {
		return "zstd"
	}
	if strings.Contains(acceptEncoding, "gzip") {
		return "gzip"
	}
	return ""
}

// Compress adds transparent response compression for clients that advertise support
// via Accept-Encoding. zstd is preferred over gzip for better compression ratios.
// WebSocket upgrade requests are passed through unmodified. For long-lived streaming
// responses, the compression buffer is flushed on every Flush() call so clients
// receive frames in real time.
func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding := negotiateEncoding(r.Header.Get("Accept-Encoding"))
		if encoding == "" {
			next.ServeHTTP(w, r)
			return
		}

		// WebSocket upgrades must not be wrapped
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r)
			return
		}

		// All /api/* requests are ConnectRPC / gRPC endpoints.  Those protocols
		// manage their own message-level framing and compression internally.
		// Applying HTTP-level gzip on top breaks the client frame parser and
		// produces binary garbage (the "Unexpected token ''" / "not valid JSON"
		// error visible in the browser).  Pass all API requests through unmodified.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Add("Vary", "Accept-Encoding")

		cw := &compressResponseWriter{
			ResponseWriter: w,
			encoding:       encoding,
		}
		defer cw.close()

		next.ServeHTTP(cw, r)
	})
}
