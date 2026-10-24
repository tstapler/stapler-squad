import { collectPerfDiagnostics } from "../perfDiagnostics";

const leaf = (id: string, viewKind: string, activeTab = "terminal") => ({ type: "leaf", id, viewKind, sessionId: id, activeTab });

describe("collectPerfDiagnostics", () => {
  afterEach(() => localStorage.clear());

  it("counts panes, terminal panes and tabs per window from the persisted layout", async () => {
    localStorage.setItem(
      "cockpit.windowLayout",
      JSON.stringify({
        version: 2,
        revision: 1,
        windows: [
          {
            id: "w0",
            name: "w0",
            paneState: { root: { type: "split", first: leaf("a", "session-list"), second: { type: "split", first: leaf("b", "session-detail"), second: leaf("c", "session-detail", "diff") } } },
          },
          { id: "w1", name: "w1", paneState: { root: leaf("d", "session-detail") } },
        ],
      })
    );
    const d = await collectPerfDiagnostics();
    expect(d.windows).toEqual([
      { id: "w0", panes: 3, terminalPanes: 2, activeTabs: { terminal: 1, diff: 1 } },
      { id: "w1", panes: 1, terminalPanes: 1, activeTabs: { terminal: 1 } },
    ]);
  });

  it("tolerates a missing or corrupt layout", async () => {
    expect((await collectPerfDiagnostics()).windows).toEqual([]);
    localStorage.setItem("cockpit.windowLayout", "{not json");
    expect((await collectPerfDiagnostics()).windows).toEqual([]);
  });
});
