// Type declarations for lzma-js library
// lzma-js uses a global LZMA object, not ES module exports

declare module 'lzma-js/src/lzma' {
  // This module adds properties to the global LZMA object
  const content: any;
  export = content;
}

declare module 'lzma-js/src/lzma.shim' {
  // This module adds iStream and oStream to the global LZMA object
  const content: any;
  export = content;
}
