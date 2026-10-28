import { decodeFrame, encodeFrame, type MuxFrame } from "../unary-mux-frame";

export class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  readonly url: string;
  readonly ctorArgs: unknown[];
  binaryType = "blob";
  sent: Uint8Array[] = [];
  closed = false;
  onopen: (() => void) | null = null;
  onmessage: ((e: { data: ArrayBuffer }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((e: { code: number }) => void) | null = null;

  constructor(...args: unknown[]) {
    this.url = args[0] as string;
    this.ctorArgs = args;
    FakeWebSocket.instances.push(this);
  }
  send(data: Uint8Array) {
    this.sent.push(data);
  }
  close() {
    this.closed = true;
  }
  open() {
    this.onopen?.();
  }
  receive(frame: MuxFrame) {
    const b = encodeFrame(frame);
    this.onmessage?.({ data: b.buffer.slice(b.byteOffset, b.byteOffset + b.byteLength) as ArrayBuffer });
  }
  serverClose(code = 1006) {
    this.onclose?.({ code });
  }
  frames(): MuxFrame[] {
    return this.sent.map((s) => decodeFrame(s));
  }
  requestIds(): number[] {
    return this.frames().flatMap((f) => (f.type === "REQUEST" ? [f.id] : []));
  }
}
