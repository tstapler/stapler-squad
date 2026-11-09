import { decideVisibilityResync, VISIBILITY_RESYNC_FRESH_MS } from "../resyncPolicy";

const term = { cols: 80, rows: 24 };

describe("decideVisibilityResync", () => {
  it("skips when the terminal cannot be measured", () => {
    expect(decideVisibilityResync(null, null, 0)).toBe("skipped_unavailable");
  });

  it("sends when nothing has synced yet", () => {
    expect(decideVisibilityResync(term, null, 0)).toBe("send");
  });

  it("skips a same-size terminal that synced recently", () => {
    const sync = { ...term, at: 1000 };
    expect(decideVisibilityResync(term, sync, 1000 + VISIBILITY_RESYNC_FRESH_MS - 1)).toBe("skipped_fresh");
  });

  it("sends once the last sync is old", () => {
    const sync = { ...term, at: 1000 };
    expect(decideVisibilityResync(term, sync, 1000 + VISIBILITY_RESYNC_FRESH_MS)).toBe("send");
  });

  it("sends when the size differs from the last synced size, however recent", () => {
    const sync = { cols: 74, rows: 30, at: 1000 };
    expect(decideVisibilityResync(term, sync, 1001)).toBe("send");
  });
});
