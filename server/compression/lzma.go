package compression

import (
	"bytes"
	"fmt"

	"github.com/ulikunitz/xz"
)

// CompressionThreshold defines minimum bytes before compression is applied
const CompressionThreshold = 1024 // 1KB - small messages not worth compressing

// CompressLZMA compresses data using LZMA algorithm (xz format)
// Only compresses if data exceeds CompressionThreshold
// Returns compressed data and compression ratio, or original data if compression not beneficial
func CompressLZMA(data []byte) ([]byte, float64, error) {
	// Skip compression for small payloads
	if len(data) < CompressionThreshold {
		return data, 1.0, nil // ratio 1.0 = no compression
	}

	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create LZMA writer: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return nil, 0, fmt.Errorf("failed to write data: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, 0, fmt.Errorf("failed to close LZMA writer: %w", err)
	}

	compressed := buf.Bytes()
	ratio := float64(len(compressed)) / float64(len(data))

	// Only return compressed data if it's actually smaller
	// Sometimes compression can make data larger for already-compressed or random data
	if len(compressed) >= len(data) {
		return data, 1.0, nil // Return original if compression didn't help
	}

	return compressed, ratio, nil
}

// DecompressLZMA decompresses LZMA-compressed data (xz format)
func DecompressLZMA(data []byte) ([]byte, error) {
	r, err := xz.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create LZMA reader: %w", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return buf.Bytes(), nil
}
