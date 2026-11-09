import { createScrollbarUpdater } from "../scrollbarUpdater";

function fakeBuffer(length: number, viewportY: number, rows: number) {
  return { buffer: { active: { length, viewportY } }, rows } as never;
}

function setup() {
  const writes: string[] = [];
  const style = (name: string) =>
    new Proxy({} as Record<string, string>, {
      set(t, k: string, v: string) {
        writes.push(`${name}.${k}=${v}`);
        t[k] = v;
        return true;
      },
    });
  let clientHeightReads = 0;
  const track = {
    style: style("track"),
    get clientHeight() {
      clientHeightReads++;
      return 200;
    },
  } as unknown as HTMLElement;
  const thumb = { style: style("thumb") } as unknown as HTMLElement;
  const frames: Array<() => void> = [];
  const updater = createScrollbarUpdater({
    getTrack: () => track,
    getThumb: () => thumb,
    requestFrame: (cb) => {
      frames.push(cb);
      return frames.length;
    },
    cancelFrame: () => {},
  });
  const runFrame = () => frames.splice(0).forEach((f) => f());
  return { updater, writes, runFrame, frames, reads: () => clientHeightReads };
}

describe("createScrollbarUpdater", () => {
  it("applies once per frame however many writes were parsed", () => {
    const { updater, writes, runFrame, frames } = setup();
    for (let i = 0; i < 50; i++) updater.schedule(fakeBuffer(1000 + i, 900, 40));
    expect(frames).toHaveLength(1);
    expect(writes).toEqual([]);
    runFrame();
    expect(writes.filter((w) => w.startsWith("thumb.height"))).toHaveLength(1);
  });

  it("measures with the newest terminal state, not the first scheduled", () => {
    const { updater, writes, runFrame } = setup();
    updater.schedule(fakeBuffer(100, 0, 50));
    updater.schedule(fakeBuffer(400, 0, 50));
    runFrame();
    // thumb height = max(20, track 200 * 50/400) = 25
    expect(writes).toContain("thumb.height=25px");
  });

  it("does not touch the DOM when nothing changed since the last apply", () => {
    const { updater, writes, runFrame } = setup();
    updater.schedule(fakeBuffer(1000, 900, 40));
    runFrame();
    writes.length = 0;
    updater.schedule(fakeBuffer(1000, 900, 40));
    runFrame();
    expect(writes).toEqual([]);
  });

  it("reads layout once, then uses the cached track height until invalidated", () => {
    const { updater, runFrame, reads } = setup();
    updater.schedule(fakeBuffer(1000, 900, 40));
    runFrame();
    updater.schedule(fakeBuffer(1001, 900, 40));
    runFrame();
    updater.schedule(fakeBuffer(1002, 900, 40));
    runFrame();
    expect(reads()).toBe(1);
    updater.invalidateTrackHeight();
    updater.schedule(fakeBuffer(1003, 900, 40));
    runFrame();
    expect(reads()).toBe(2);
  });

  it("hides the track when everything fits", () => {
    const { updater, writes, runFrame } = setup();
    updater.schedule(fakeBuffer(40, 0, 40));
    runFrame();
    expect(writes).toEqual(["track.display=none"]);
  });

  it("flush applies immediately and cancels the pending frame", () => {
    const { updater, writes } = setup();
    updater.schedule(fakeBuffer(1000, 0, 40));
    updater.flush();
    expect(writes.some((w) => w.startsWith("thumb.height"))).toBe(true);
  });
});
