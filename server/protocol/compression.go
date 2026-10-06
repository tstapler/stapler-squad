package protocol

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	kgzip "github.com/klauspost/compress/gzip"

	"github.com/tstapler/stapler-squad/log"
)

// compressionPool holds reusable gzip.Writer instances scoped to envelope-level payload
// compression (terminal-resync-reliability Epic 5.1, Task 5.1.1.1). This is a separate pool
// from server/middleware/gzip.go's HTTP-response compression pool: that pool's writers are
// Reset() against an http.ResponseWriter on every request, so sharing it here would put this
// path's writer a Reset() away from writing into an in-flight HTTP response from a completely
// unrelated request.
var compressionPool = sync.Pool{
	New: func() any {
		gz, _ := kgzip.NewWriterLevel(io.Discard, kgzip.DefaultCompression)
		return gz
	},
}

// CompressEnvelopeIfLarge gzip-compresses payload when it exceeds threshold bytes, returning
// the compressed bytes and wasCompressed=true. Payloads at or under the threshold are returned
// unmodified with wasCompressed=false so callers can decide whether to set the envelope's
// CompressedFlag (see CompressedFlag, above) before writing the frame.
//
// server/services/connectrpc_websocket.go's writeCurrentPaneResponse is the shared
// envelope-writing helper that calls this; all CurrentPaneRequest response paths route through
// it so compression is applied uniformly.
func CompressEnvelopeIfLarge(payload []byte, threshold int) (compressed []byte, wasCompressed bool, err error) {
	if len(payload) <= threshold {
		return payload, false, nil
	}

	gz, ok := compressionPool.Get().(*kgzip.Writer)
	if !ok {
		return nil, false, fmt.Errorf("compressionPool returned unexpected type")
	}
	defer compressionPool.Put(gz)

	var buf bytes.Buffer
	gz.Reset(&buf)
	if _, err := gz.Write(payload); err != nil {
		return nil, false, fmt.Errorf("failed to gzip envelope payload: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, false, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	log.Debug("compressed envelope payload",
		"pre_compression_bytes", len(payload),
		"post_compression_bytes", buf.Len(),
	)

	return buf.Bytes(), true, nil
}
