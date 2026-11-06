// Mounts the production CmEngine on a lazy buffer and exposes it through the WindowedEngine seam the host uses.
import { CmEngine } from "@/components/sessions/files/engine/CmEngine";
import type { EngineHandle } from "@/components/sessions/files/engine/EditorEngine";
import type { WindowedEngine } from "@/lib/files/windowedFile";

export interface MountedLazy { handle: EngineHandle; adapter: WindowedEngine }

export async function mountLazyEngine(host: HTMLElement): Promise<MountedLazy> {
  const engine = new CmEngine();
  const config = { mode: "Read" as const, ariaLabel: "File: big.txt" };
  const handle = engine.mount(host, { buffer: await engine.createBuffer({ doc: "", lazy: { lineCount: 1 }, config }), ...config });
  const adapter: WindowedEngine = {
    createBufferState: (init) => engine.createBuffer({ ...init, config }),
    setBufferState: (s) => handle.setBuffer(s),
    onNeedRange: (cb) => handle.onNeedRange(cb),
    provideRange: (r, t) => handle.provideRange(r, t),
    gotoLine: (n) => handle.reveal(n),
    getCursor: () => handle.getCursor(),
    highlightRange: (r) => handle.highlight(r),
    getViewport: () => handle.getViewport(),
  };
  return { handle, adapter };
}
