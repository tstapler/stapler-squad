declare module 'lzma-js/src/lzma' {
  export class iStream {
    constructor(data: Uint8Array | ArrayBuffer);
  }

  export class oStream {
    toUint8Array(): Uint8Array;
    toString(): string;
  }

  export function decompressFile(inStream: iStream, outStream?: oStream): oStream;

  export function decompress(
    properties: any,
    inStream: iStream,
    outStream: oStream,
    outSize: number
  ): void;

  export class Decoder {
    decodeHeader(inStream: iStream): any;
    setProperties(header: any): void;
    decodeBody(inStream: iStream, outStream: oStream, maxSize: number): boolean;
  }
}
