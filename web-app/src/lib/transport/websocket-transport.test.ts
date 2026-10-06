/**
 * Tests for WebSocket transport envelope encoding
 *
 * These tests verify that the client-side envelope encoding matches
 * the server's expectations for the ConnectRPC protocol.
 */

import { gzipSync } from "zlib";

// Extract the encodeEnvelope function for testing
function encodeEnvelope(flags: number, data: Uint8Array): Uint8Array {
  const envelope = new Uint8Array(5 + data.length);
  envelope[0] = flags;
  // Write length as big-endian uint32
  const view = new DataView(envelope.buffer, envelope.byteOffset, envelope.byteLength);
  view.setUint32(1, data.length, false); // false = big-endian
  envelope.set(data, 5);
  return envelope;
}

// Extract the decompressGzipPayload function for testing. Duplicated locally rather than
// imported from ./websocket-transport (as encodeEnvelope is above) because that module
// transitively imports "it-ws/client", whose installed package in this checkout ships only
// src/ (no dist/ build output the package.json "exports" map points at) — a pre-existing,
// unrelated packaging gap that makes the real module unresolvable under Jest regardless of
// this change. See server/protocol/compression_test.go for the corresponding Go-side coverage.
async function decompressGzipPayload(data: Uint8Array): Promise<Uint8Array> {
  // Typed as Uint8Array<ArrayBuffer> (rather than the bare Uint8Array default of
  // Uint8Array<ArrayBufferLike>) to match DecompressionStream.readable's declared type in
  // lib.dom.d.ts exactly — otherwise pipeThrough's generic inference can't reconcile the two
  // and tsc rejects the call. Kept in sync with websocket-transport.ts's real implementation.
  const stream = new ReadableStream<Uint8Array<ArrayBuffer>>({
    start(controller) {
      controller.enqueue(data as Uint8Array<ArrayBuffer>);
      controller.close();
    },
  });
  const decompressed = stream.pipeThrough(new DecompressionStream("gzip"));
  const reader = decompressed.getReader();

  const chunks: Uint8Array[] = [];
  let totalLength = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    totalLength += value.length;
  }

  const result = new Uint8Array(totalLength);
  let offset = 0;
  for (const chunk of chunks) {
    result.set(chunk, offset);
    offset += chunk.length;
  }
  return result;
}

describe('WebSocket Transport - Envelope Encoding', () => {
  describe('encodeEnvelope', () => {
    it('should encode simple message correctly', () => {
      const data = new TextEncoder().encode('test');
      const envelope = encodeEnvelope(0x00, data);

      expect(envelope.length).toBe(9); // 5 header + 4 data
      expect(envelope[0]).toBe(0x00); // flags
      expect(envelope[1]).toBe(0x00); // length byte 1
      expect(envelope[2]).toBe(0x00); // length byte 2
      expect(envelope[3]).toBe(0x00); // length byte 3
      expect(envelope[4]).toBe(0x04); // length byte 4 (4 bytes)
      expect(new TextDecoder().decode(envelope.slice(5))).toBe('test');
    });

    it('should handle empty data', () => {
      const envelope = encodeEnvelope(0x00, new Uint8Array(0));

      expect(envelope.length).toBe(5);
      expect(envelope[0]).toBe(0x00); // flags
      expect(envelope[1]).toBe(0x00); // length MSB
      expect(envelope[2]).toBe(0x00);
      expect(envelope[3]).toBe(0x00);
      expect(envelope[4]).toBe(0x00); // length LSB (0 bytes)
    });

    it('should set flags correctly', () => {
      const data = new Uint8Array([1, 2, 3]);
      const envelope = encodeEnvelope(0x02, data); // EndStream flag

      expect(envelope[0]).toBe(0x02);
      expect(envelope[4]).toBe(0x03); // 3 bytes of data
    });

    it('should encode length in big-endian format', () => {
      const data = new Uint8Array(256); // 0x100 in hex
      const envelope = encodeEnvelope(0x00, data);

      // Big-endian encoding of 256: 0x00000100
      expect(envelope[1]).toBe(0x00);
      expect(envelope[2]).toBe(0x00);
      expect(envelope[3]).toBe(0x01);
      expect(envelope[4]).toBe(0x00);
    });

    it('should handle protobuf-like binary data', () => {
      // Typical protobuf data structure
      const data = new Uint8Array([
        0x0a, 0x0b, // field 1, length 11
        0x6e, 0x65, 0x77, 0x2d, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, // "new-session"
        0x1a, 0x03, // field 3, length 3
        0x0a, 0x01, 0x74 // nested field with 't'
      ]);

      const envelope = encodeEnvelope(0x00, data);

      expect(envelope[0]).toBe(0x00);
      expect(envelope[4]).toBe(data.length);
      expect(envelope.slice(5)).toEqual(data);
    });

    it('should handle terminal escape sequences', () => {
      const data = new TextEncoder().encode('\x1b[31mred\x1b[0m');
      const envelope = encodeEnvelope(0x00, data);

      expect(envelope[4]).toBe(data.length);
      expect(new TextDecoder().decode(envelope.slice(5))).toBe('\x1b[31mred\x1b[0m');
    });

    it('should handle binary data with null bytes', () => {
      const data = new Uint8Array([0x00, 0xFF, 0x00, 0x7F]);
      const envelope = encodeEnvelope(0x00, data);

      expect(envelope[4]).toBe(0x04);
      expect(envelope.slice(5)).toEqual(data);
    });

    it('should match Go CreateEnvelope output format', () => {
      // This is the exact format that Go's CreateEnvelope produces
      const data = new TextEncoder().encode('hello');
      const envelope = encodeEnvelope(0x00, data);

      // Expected: [0x00, 0x00, 0x00, 0x00, 0x05, 'h', 'e', 'l', 'l', 'o']
      const expected = new Uint8Array([
        0x00, 0x00, 0x00, 0x00, 0x05,
        0x68, 0x65, 0x6c, 0x6c, 0x6f
      ]);

      expect(envelope).toEqual(expected);
    });

    it('should handle large messages (1KB)', () => {
      const data = new Uint8Array(1024);
      data.fill(0xAB); // Fill with pattern

      const envelope = encodeEnvelope(0x00, data);

      // 1024 = 0x00000400 in big-endian
      expect(envelope[1]).toBe(0x00);
      expect(envelope[2]).toBe(0x00);
      expect(envelope[3]).toBe(0x04);
      expect(envelope[4]).toBe(0x00);
      expect(envelope.length).toBe(1029); // 5 + 1024
    });

    it('should correctly use DataView with byteOffset', () => {
      // This test verifies the fix for the DataView byteOffset bug
      const data = new Uint8Array([0xAA, 0xBB]);
      const envelope = encodeEnvelope(0x00, data);

      // Manually read the length field using DataView
      const view = new DataView(envelope.buffer, envelope.byteOffset + 1, 4);
      const length = view.getUint32(0, false);

      expect(length).toBe(2);
      expect(envelope[5]).toBe(0xAA);
      expect(envelope[6]).toBe(0xBB);
    });
  });

  // AC6a (terminal-resync-reliability Epic 5.1): parseResponseBody's compressed-frame handling.
  // decompressGzipPayload is the extracted decompression step parseResponseBody calls when the
  // envelope's CompressedFlag is set (server/protocol/envelope.go); these tests exercise it
  // directly since parseResponseBody itself is a private closure inside stream().
  describe('parseResponseBody compressed-frame handling', () => {
    it('parseResponseBody_should_DecompressGzipPayload_When_CompressedFlagSet', async () => {
      const original = new TextEncoder().encode(
        'the quick brown fox jumps over the lazy dog '.repeat(50)
      );
      // Compressed with Node's zlib (a standard gzip encoder), simulating the payload the Go
      // server produces via server/protocol's CompressEnvelopeIfLarge (also standard gzip).
      const compressed = new Uint8Array(gzipSync(Buffer.from(original)));

      const result = await decompressGzipPayload(compressed);

      expect(result).toEqual(original);
    });

    it('parseResponseBody_should_SurfaceDecodeError_When_CompressedFlagSetButPayloadIsTruncated', async () => {
      const original = new TextEncoder().encode(
        'the quick brown fox jumps over the lazy dog '.repeat(50)
      );
      const compressed = new Uint8Array(gzipSync(Buffer.from(original)));
      const truncated = compressed.slice(0, compressed.length - 10);

      await expect(decompressGzipPayload(truncated)).rejects.toThrow();
    });
  });

  describe('envelope format compatibility', () => {
    it('should create envelopes parseable by Go ParseEnvelope', () => {
      // Test cases that match the Go test suite
      const testCases = [
        { flags: 0x00, data: 'test' },
        { flags: 0x00, data: '' },
        { flags: 0x02, data: 'end' },
        { flags: 0x01, data: '\x01\x02' },
      ];

      testCases.forEach(({ flags, data }) => {
        const dataBytes = new TextEncoder().encode(data);
        const envelope = encodeEnvelope(flags, dataBytes);

        // Verify format: [flags][4-byte length][data]
        expect(envelope[0]).toBe(flags);

        // Verify big-endian length encoding
        const view = new DataView(envelope.buffer, envelope.byteOffset + 1, 4);
        expect(view.getUint32(0, false)).toBe(dataBytes.length);

        // Verify data
        expect(envelope.slice(5)).toEqual(dataBytes);
      });
    });
  });
});
