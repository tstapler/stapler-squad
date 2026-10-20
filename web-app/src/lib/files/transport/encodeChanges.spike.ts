// SPIKE (Task 0.1.8a, ADR-036 S-T4), promoted to encodeChanges.ts by Task 2.2.5a.
//
// Converts a CodeMirror ChangeSet over the LF-normalized client text into position-addressed edits in BASE
// coordinates, and carries the TS reference applier that mirrors the server's byte-level rules:
// line/col_utf16 -> byte offsets, surrogate-pair splits rejected, EOL and BOM owned by the applier.
//
// Golden-vector schema for testdata/transport/*.json (fixed here, consumed by Task 2.1.5f). One file holds
// {"vectors": GoldenVector[]}; bytes are lowercase hex; "error" is null on success, else one of EditError:
//   { "name": string, "base_hex": string,            // file bytes: optional EF BB BF BOM, LF or CRLF line ends
//     "edits": [ { "start": {"line": n, "col_utf16": n}, "end": {...}, "insert": string, "deleted_crc32c": uint32 } ],
//     "result_hex": string | null, "error": EditError | null }
// Lines are 0-based; col_utf16 counts UTF-16 code units of the line text excluding its EOL; "insert" uses "\n"
// and the applier rewrites it to the file's EOL; deleted_crc32c covers the deleted range's bytes as stored in the file.
import { ChangeSet, Text } from "@codemirror/state";

export interface Pos {
  line: number;
  col_utf16: number;
}
export interface Edit {
  start: Pos;
  end: Pos;
  insert: string;
  deleted_crc32c: number;
}
export type EditError = "surrogate_split" | "crc_mismatch" | "out_of_range" | "overlap";
export interface GoldenVector {
  name: string;
  base_hex: string;
  edits: Edit[];
  result_hex: string | null;
  error: EditError | null;
}

export class EditRejected extends Error {
  constructor(readonly kind: EditError, detail = "") {
    super(`${kind}${detail ? `: ${detail}` : ""}`);
  }
}

const BOM = [0xef, 0xbb, 0xbf];
const enc = new TextEncoder();
const dec = new TextDecoder("utf-8", { fatal: true });

const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? (c >>> 1) ^ 0x82f63b78 : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

export function crc32c(bytes: Uint8Array): number {
  let c = 0xffffffff;
  for (const b of bytes) c = CRC_TABLE[(c ^ b) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

export interface FileShape {
  bom: boolean;
  eol: "\n" | "\r\n";
}

export function fileShape(bytes: Uint8Array): FileShape {
  const bom = BOM.every((b, i) => bytes[i] === b);
  const body = bom ? bytes.subarray(3) : bytes;
  const lf = body.indexOf(0x0a);
  return { bom, eol: lf > 0 && body[lf - 1] === 0x0d ? "\r\n" : "\n" };
}

/** Client text for a file: BOM stripped, line ends normalized to "\n" (what CodeMirror holds). */
export function clientText(bytes: Uint8Array): string {
  const { bom } = fileShape(bytes);
  return dec.decode(bom ? bytes.subarray(3) : bytes).replace(/\r\n/g, "\n");
}

/** Encoder: ChangeSet over `base` (LF-normalized) -> edits in base coordinates; crc covers the deleted bytes in file form. */
export function encodeChanges(changes: ChangeSet, base: string, shape: FileShape): Edit[] {
  const doc = Text.of(base.split("\n"));
  const toPos = (pos: number): Pos => {
    const line = doc.lineAt(pos);
    return { line: line.number - 1, col_utf16: pos - line.from };
  };
  const edits: Edit[] = [];
  changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
    const deleted = base.slice(fromA, toA).replace(/\n/g, shape.eol);
    edits.push({ start: toPos(fromA), end: toPos(toA), insert: inserted.toString(), deleted_crc32c: crc32c(enc.encode(deleted)) });
  });
  return edits;
}

interface LineIndex {
  starts: number[];
  ends: number[];
  texts: string[];
}

function indexLines(body: Uint8Array, eol: string): LineIndex {
  const eolLen = eol.length;
  const starts: number[] = [];
  const ends: number[] = [];
  const texts: string[] = [];
  let start = 0;
  for (let i = 0; i <= body.length; i++) {
    const atEnd = i === body.length;
    const isEol = !atEnd && body[i] === 0x0a;
    if (!atEnd && !isEol) continue;
    const end = isEol && eolLen === 2 ? i - 1 : i;
    starts.push(start);
    ends.push(end);
    texts.push(dec.decode(body.subarray(start, end)));
    start = i + 1;
  }
  return { starts, ends, texts };
}

function byteOffset(ix: LineIndex, pos: Pos): number {
  if (pos.line < 0 || pos.line >= ix.texts.length) throw new EditRejected("out_of_range", `line ${pos.line}`);
  const text = ix.texts[pos.line];
  if (pos.col_utf16 < 0 || pos.col_utf16 > text.length) throw new EditRejected("out_of_range", `col ${pos.col_utf16}`);
  const before = text.charCodeAt(pos.col_utf16 - 1);
  const after = text.charCodeAt(pos.col_utf16);
  if (before >= 0xd800 && before <= 0xdbff && after >= 0xdc00 && after <= 0xdfff) throw new EditRejected("surrogate_split");
  return ix.starts[pos.line] + enc.encode(text.slice(0, pos.col_utf16)).length;
}

/** Reference applier: bytes in, bytes out; EOL and BOM are preserved from the base file, never taken from `insert`. */
export function applyEdits(file: Uint8Array, edits: Edit[]): Uint8Array {
  const { bom, eol } = fileShape(file);
  const body = bom ? file.subarray(3) : file;
  const ix = indexLines(body, eol);
  const spans = edits.map((e) => ({ from: byteOffset(ix, e.start), to: byteOffset(ix, e.end), e }));
  const out: number[] = bom ? [...BOM] : [];
  let cursor = 0;
  for (const { from, to, e } of spans) {
    if (from < cursor || to < from) throw new EditRejected("overlap");
    if (crc32c(body.subarray(from, to)) !== e.deleted_crc32c) throw new EditRejected("crc_mismatch");
    for (let i = cursor; i < from; i++) out.push(body[i]);
    for (const b of enc.encode(e.insert.replace(/\n/g, eol))) out.push(b);
    cursor = to;
  }
  for (let i = cursor; i < body.length; i++) out.push(body[i]);
  return Uint8Array.from(out);
}

/** Full-replace oracle: apply the ChangeSet to the whole client text, then re-apply EOL and BOM. */
export function fullReplace(file: Uint8Array, changes: ChangeSet): Uint8Array {
  const { bom, eol } = fileShape(file);
  const next = changes.apply(Text.of(clientText(file).split("\n"))).toString().replace(/\n/g, eol);
  const body = enc.encode(next);
  return bom ? Uint8Array.from([...BOM, ...body]) : body;
}

export const toHex = (b: Uint8Array): string => Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
export const fromHex = (h: string): Uint8Array => Uint8Array.from(h.match(/../g) ?? [], (x) => Number.parseInt(x, 16));

export function goldenVector(name: string, file: Uint8Array, changes: ChangeSet): GoldenVector {
  const edits = encodeChanges(changes, clientText(file), fileShape(file));
  try {
    return { name, base_hex: toHex(file), edits, result_hex: toHex(applyEdits(file, edits)), error: null };
  } catch (e) {
    if (e instanceof EditRejected) return { name, base_hex: toHex(file), edits, result_hex: null, error: e.kind };
    throw e;
  }
}
