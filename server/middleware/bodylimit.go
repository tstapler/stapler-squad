package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// BodyLimit caps the request body of one procedure path (as the wrapped handler sees
// it, after any prefix strip), on the wire and after gzip inflation. Reading past max
// fails with *http.MaxBytesError, which connect-go reports as ResourceExhausted before
// the RPC handler runs. Other paths pass untouched.
//
// connect-go inflates a gzip unary body itself with no size bound, so a few KB on the
// wire could become hundreds of MB; the body is inflated here under the same cap instead
// and the Content-Encoding header is dropped so connect sees an identity body.
func BodyLimit(path string, limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == path {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
				if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
					r.Body = http.MaxBytesReader(w, &gzipBody{src: r.Body}, limit)
					r.Header.Del("Content-Encoding")
					r.ContentLength = -1
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// gzipBody inflates src on first read, so a bad header surfaces as a read error.
type gzipBody struct {
	src io.ReadCloser
	zr  *gzip.Reader
}

func (b *gzipBody) Read(p []byte) (int, error) {
	if b.zr == nil {
		zr, err := gzip.NewReader(b.src)
		if err != nil {
			return 0, err
		}
		b.zr = zr
	}
	return b.zr.Read(p)
}

func (b *gzipBody) Close() error {
	if b.zr != nil {
		_ = b.zr.Close()
	}
	return b.src.Close()
}
