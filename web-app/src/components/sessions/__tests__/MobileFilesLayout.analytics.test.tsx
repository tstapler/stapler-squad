import React from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { createFakeTransport, createFilesTestHarness } from "@/lib/files/__tests__/fixtures";
import { useFilesV2, type FilesV2 } from "@/lib/hooks/useFilesV2";
import { MobileFilesLayout } from "../files/MobileFilesLayout";

jest.mock("@/lib/hooks/useFileService", () => ({ invalidateFileContent: jest.fn() }));

const FILES = { "a.go": "x", "b.go": "y", "c.go": "z" };

function mount() {
  const h = createFilesTestHarness({ sessionId: "s1", transport: createFakeTransport({ files: FILES }) });
  let v2!: FilesV2;
  function Harness() {
    v2 = useFilesV2("s1", false, {});
    return (
      <MobileFilesLayout
        active v2={v2} pane="editor" origin="files" containerWidth={390}
        onPane={jest.fn()} onBack={jest.fn()} onQuickOpen={jest.fn()} onSearch={jest.fn()}
      >
        {null}
      </MobileFilesLayout>
    );
  }
  render(<Harness />, { wrapper: h.wrapper });
  const events = (name: string) => h.analytics.events.filter((e) => e.name === name).map((e) => e.labels);
  return { h, events, get v2() { return v2; } };
}

async function openSheet(m: ReturnType<typeof mount>) {
  for (const p of ["a.go", "b.go", "c.go"]) {
    act(() => m.v2.buffers.open(p, { pinned: true }));
    await waitFor(() => expect(m.v2.buffers.active).toMatchObject({ path: p, status: "loaded" }));
  }
  fireEvent.click(screen.getByRole("button", { name: "Open files, 3" }));
  return screen.getByTestId("tab-switcher-sheet");
}

describe("MobileFilesLayout tab analytics", () => {
  beforeEach(() => window.localStorage.clear());

  it("tabSheet_should_EmitTabSwitchViaSheet_When_ARowIsSelected", async () => {
    const m = mount();
    const sheet = await openSheet(m);

    fireEvent.click(within(sheet).getByRole("button", { name: /^a\.go/ }));

    expect(m.events("files.tab.switch")).toEqual([expect.objectContaining({ via: "sheet" })]);
  });

  it("tabSheet_should_EmitTabCloseWithMethod_When_ClosedFromRowButtonMenuOrCloseOthers", async () => {
    const m = mount();
    const sheet = await openSheet(m);

    fireEvent.click(within(sheet).getByRole("button", { name: "Close c.go" }));
    fireEvent.click(within(sheet).getByRole("button", { name: "Actions for b.go" }));
    fireEvent.click(within(sheet).getByRole("menuitem", { name: "Close" }));

    expect(m.events("files.tab.close")).toEqual([
      expect.objectContaining({ method: "button", dirty: "false" }),
      expect.objectContaining({ method: "context_menu", dirty: "false" }),
    ]);
  });

  it("tabSheet_should_EmitSingleCloseOthersEvent_When_CloseOthersChosen", async () => {
    const m = mount();
    const sheet = await openSheet(m);

    fireEvent.click(within(sheet).getByRole("button", { name: "Actions for a.go" }));
    fireEvent.click(within(sheet).getByRole("menuitem", { name: "Close others" }));

    expect(m.events("files.tab.close")).toEqual([expect.objectContaining({ method: "close_others", dirty: "false", open_tabs: "2-3" })]);
  });
});
