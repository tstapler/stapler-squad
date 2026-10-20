import { EditorView } from "@codemirror/view";
import { Text } from "@codemirror/state";
import { VirtualDoc } from "@/lib/__tests__/virtualFile";
import { CmEngine } from "../CmEngine";
import type { LazyMode } from "../cm6LazyDoc/controller";
import type { Edit, EngineHandle } from "../EditorEngine";

export interface OpenLazy {
  handle: EngineHandle;
  view: EditorView;
  file: VirtualDoc;
}

/** Mounts a lazy buffer of `lineCount` fully unloaded lines over a deterministic fake file. */
export async function openLazy(host: HTMLElement, lineCount: number, mode: LazyMode = "gap-line", seed = 7): Promise<OpenLazy> {
  const engine = new CmEngine(mode);
  const config = { mode: "Edit" as const, ariaLabel: "File: lazy.txt" };
  const buffer = await engine.createBuffer({ doc: "", lazy: { lineCount }, config });
  const handle = engine.mount(host, { buffer, ...config });
  const view = EditorView.findFromDOM(host)!;
  return { handle, view, file: new VirtualDoc(lineCount, seed) };
}

/** Supplies file lines [startLine, startLine + count) (1-based) from the fake file. */
export function provide(o: OpenLazy, startLine: number, count: number): void {
  o.handle.provideRange({ startLine, endLine: startLine + count - 1 }, o.file.fetch(startLine - 1, count).join("\n"));
}

/** Applies base-coordinate edits (ordered, non-overlapping) to `lines`, returning the edited copy. */
export function applyEdits(lines: string[], edits: Edit[]): string[] {
  const doc = Text.of(lines);
  const offset = (p: Edit["start"]) => doc.line(p.line + 1).from + p.colUtf16;
  let text = doc.toString();
  for (const e of [...edits].reverse()) text = text.slice(0, offset(e.start)) + e.insert + text.slice(offset(e.end));
  return text.split("\n");
}
