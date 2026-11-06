/**
 * Task 0.1.8a (ADR-036 S-T4): the ChangeSet -> edits encoder against a full-replace oracle. A wrong edit whose
 * checksums derive from the same wrong position would pass any self-consistency check, so the oracle is
 * independent: apply the ChangeSet to the whole text, re-apply EOL and BOM, compare bytes.
 */
import * as fc from "fast-check";
import { ChangeSet, Text } from "@codemirror/state";
import {
  EditRejected, applyEdits, clientText, crc32c, encodeChanges, fileShape, fromHex, fullReplace, goldenVector, toHex,
} from "../transport/encodeChanges.spike";

const SEED = 20261016;
const RUNS = 10_000;
const ALPHABET = ["a", "b", " ", "é", "中", "é", "\u{1F600}", "\u{1D11E}", "_", "0"];
const enc = new TextEncoder();

const lineArb = fc.array(fc.constantFrom(...ALPHABET), { maxLength: 8 }).map((p) => p.join(""));
const fileArb = fc.record({
  lines: fc.array(lineArb, { minLength: 1, maxLength: 8 }),
  crlf: fc.boolean(),
  bom: fc.boolean(),
  trailingNewline: fc.boolean(),
}).map(({ lines, crlf, bom, trailingNewline }) => {
  const eol = crlf ? "\r\n" : "\n";
  const text = lines.join(eol) + (trailingNewline ? eol : "");
  const body = enc.encode(text);
  return bom ? Uint8Array.from([0xef, 0xbb, 0xbf, ...body]) : body;
});

/** Random sorted, non-overlapping edits over [0, length], adjacent edits and both ends included. */
function changesArb(length: number): fc.Arbitrary<ChangeSet> {
  const insertArb = fc.array(fc.constantFrom(...ALPHABET, "\n"), { maxLength: 4 }).map((p) => p.join(""));
  return fc
    .array(fc.integer({ min: 0, max: length }), { maxLength: 8 })
    .chain((cuts) => {
      const sorted = [...cuts].sort((a, b) => a - b);
      const pairs: [number, number][] = [];
      for (let i = 0; i + 1 < sorted.length; i += 2) pairs.push([sorted[i], sorted[i + 1]]);
      return fc.array(insertArb, { minLength: pairs.length, maxLength: pairs.length }).map((ins) =>
        ChangeSet.of(pairs.map(([from, to], i) => ({ from, to, insert: ins[i] })), length)
      );
    });
}

function splitsSurrogate(base: string, changes: ChangeSet): boolean {
  let split = false;
  changes.iterChangedRanges((fromA, toA) => {
    for (const p of [fromA, toA]) {
      const a = base.charCodeAt(p - 1);
      const b = base.charCodeAt(p);
      if (a >= 0xd800 && a <= 0xdbff && b >= 0xdc00 && b <= 0xdfff) split = true;
    }
  });
  return split;
}

describe("ChangeSet encoder spike (S-T4)", () => {
  it("encoder_should_EqualFullReplaceBytewise_For10000SeededCases_And_RejectSurrogateSplits", () => {
    let rejected = 0;
    let applied = 0;
    fc.assert(
      fc.property(
        fileArb.chain((file) => {
          const len = clientText(file).length;
          return changesArb(len).map((changes) => ({ file, changes }));
        }),
        ({ file, changes }) => {
          const base = clientText(file);
          const edits = encodeChanges(changes, base, fileShape(file));
          if (splitsSurrogate(base, changes)) {
            expect(() => applyEdits(file, edits)).toThrow(EditRejected);
            rejected++;
            return;
          }
          expect(toHex(applyEdits(file, edits))).toBe(toHex(fullReplace(file, changes)));
          applied++;
        }
      ),
      { numRuns: RUNS, seed: SEED, verbose: 1 }
    );
    console.log(`SPIKE 0.1.8a seed=${SEED} runs=${RUNS} applied=${applied} surrogateRejected=${rejected}`);
    expect(applied + rejected).toBe(RUNS);
    expect(rejected).toBeGreaterThan(0);
  });

  it("applier_should_RejectWrongChecksum_And_OutOfRange", () => {
    const file = enc.encode("ab\r\ncd");
    const changes = ChangeSet.of([{ from: 1, to: 4, insert: "X" }], 5);
    const edits = encodeChanges(changes, clientText(file), fileShape(file));
    expect(toHex(applyEdits(file, edits))).toBe(toHex(enc.encode("aXd")));
    expect(() => applyEdits(file, [{ ...edits[0], deleted_crc32c: edits[0].deleted_crc32c ^ 1 }])).toThrow(/crc_mismatch/);
    expect(() => applyEdits(file, [{ ...edits[0], end: { line: 9, col_utf16: 0 } }])).toThrow(/out_of_range/);
    expect(crc32c(enc.encode("123456789"))).toBe(0xe3069283);
  });

  it("goldenVector_should_RoundTripThroughJsonSchema", () => {
    const file = Uint8Array.from([0xef, 0xbb, 0xbf, ...enc.encode("héllo\r\n\u{1F600}x\r\n")]);
    const changes = ChangeSet.of([{ from: 0, to: 2, insert: "J" }, { from: 8, insert: "!\n" }], Text.of(clientText(file).split("\n")).length);
    const vec = JSON.parse(JSON.stringify({ vectors: [goldenVector("bom-crlf-multibyte", file, changes)] })).vectors[0];
    expect(Object.keys(vec).sort()).toEqual(["base_hex", "edits", "error", "name", "result_hex"]);
    expect(vec.error).toBeNull();
    expect(Object.keys(vec.edits[0]).sort()).toEqual(["deleted_crc32c", "end", "insert", "start"]);
    expect(Object.keys(vec.edits[0].start).sort()).toEqual(["col_utf16", "line"]);
    expect(toHex(fromHex(vec.base_hex))).toBe(vec.base_hex);
    expect(vec.result_hex).toBe(toHex(fullReplace(file, changes)));
  });
});
