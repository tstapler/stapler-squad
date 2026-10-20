// CRC32C (Castagnoli, reflected polynomial 0x82F63B78), matching Go's crc32.Castagnoli.
const TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? (c >>> 1) ^ 0x82f63b78 : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

/** Pass the previous result as `seed` to checksum data in pieces. */
export function crc32c(bytes: Uint8Array, seed = 0): number {
  let c = ~seed >>> 0;
  for (let i = 0; i < bytes.length; i++) c = TABLE[(c ^ bytes[i]) & 0xff] ^ (c >>> 8);
  return ~c >>> 0;
}
