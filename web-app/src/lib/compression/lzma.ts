// Import lzma-js library (loads both lzma.js and lzma.shim.js via global LZMA object)
// Note: lzma-js uses a global LZMA object, not ES module exports
// We need to declare it as a global type and load via dynamic import
declare global {
  interface Window {
    LZMA: {
      iStream: new (buffer: ArrayBuffer | Uint8Array) => {
        readByte: () => number;
        size: number;
        offset: number;
      };
      oStream: new () => {
        toUint8Array: () => Uint8Array;
        toString: () => string;
      };
      decompressFile: (inStream: any, outStream?: any) => any;
    };
  }
}

// Load lzma-js library dynamically
let lzmaLoaded = false;
async function ensureLZMALoaded(): Promise<void> {
  if (lzmaLoaded) return;

  if (typeof window === 'undefined') {
    throw new Error('LZMA decompression is only available in browser environment');
  }

  // Import both lzma.js and lzma.shim.js
  await import('lzma-js/src/lzma');
  await import('lzma-js/src/lzma.shim');

  lzmaLoaded = true;
}

/**
 * Decompresses LZMA-compressed data (xz format)
 * Compatible with server-side compression from github.com/ulikunitz/xz
 */
export async function decompressLZMA(data: Uint8Array): Promise<Uint8Array> {
  return new Promise(async (resolve, reject) => {
    try {
      // Ensure LZMA library is loaded
      await ensureLZMALoaded();

      // Access the global LZMA object
      const LZMA = (window as any).LZMA;

      if (!LZMA || !LZMA.iStream || !LZMA.decompressFile) {
        throw new Error('LZMA library not properly loaded');
      }

      // Create input stream from Uint8Array
      // LZMA.iStream expects ArrayBuffer, so convert if needed
      const buffer = data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength);
      const inStream = new LZMA.iStream(buffer);

      // Decompress the file
      const outStream = LZMA.decompressFile(inStream);

      // Convert output stream to Uint8Array
      const result = outStream.toUint8Array();

      resolve(result);
    } catch (err) {
      reject(err instanceof Error ? err : new Error(String(err)));
    }
  });
}

/**
 * Detects if data is LZMA-compressed by checking for XZ magic bytes
 * XZ format starts with: 0xFD 0x37 0x7A 0x58 0x5A 0x00
 */
export function isLZMACompressed(data: Uint8Array): boolean {
  if (data.length < 6) {
    return false;
  }

  // Check for XZ magic number
  return (
    data[0] === 0xFD &&
    data[1] === 0x37 &&
    data[2] === 0x7A &&
    data[3] === 0x58 &&
    data[4] === 0x5A &&
    data[5] === 0x00
  );
}
