import * as LZMA from 'lzma-js/src/lzma';

/**
 * Decompresses LZMA-compressed data (xz format)
 * Compatible with server-side compression from github.com/ulikunitz/xz
 */
export async function decompressLZMA(data: Uint8Array): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    try {
      // Create input stream from Uint8Array
      const inStream = new LZMA.iStream(data);

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
