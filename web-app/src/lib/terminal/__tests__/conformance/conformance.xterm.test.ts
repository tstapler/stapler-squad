import { Terminal } from "@xterm/xterm";
import { describeConformance } from "./runBaseline";
import { normalizeScreen, type Vector } from "./vectors";

async function renderXterm(v: Vector) {
  const term = new Terminal({ cols: v.cols, rows: v.rows, allowProposedApi: true });
  let response = "";
  term.onData((d) => { response += d; });
  await new Promise<void>((resolve) => term.write(v.input, resolve));
  const buf = term.buffer.active;
  const rows = Array.from({ length: v.rows }, (_, y) => buf.getLine(buf.viewportY + y)?.translateToString(true) ?? "");
  term.dispose();
  return { screen: normalizeScreen(rows), response };
}

describe("VT conformance: xterm.js baseline", () => {
  describeConformance("xterm", renderXterm);
});
