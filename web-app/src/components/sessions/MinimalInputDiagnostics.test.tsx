import { act, fireEvent, render, screen } from "@testing-library/react";
import { MinimalInputDiagnostics } from "./MinimalInputDiagnostics";
import { createDiagnostics } from "@/lib/terminal/mobileInput/diagnostics";

let mockSearch = "";
jest.mock("@/lib/nav", () => ({ useSearchParams: () => new URLSearchParams(mockSearch) }));

const HEADER = { renderer: "xterm", flag: "on (server)", streamState: "connected", keyboardHint: "gboard" };

function setClipboard(writeText: jest.Mock) {
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
}

beforeEach(() => {
  mockSearch = "";
});

describe("MinimalInputDiagnostics", () => {
  it("panel_should_ShowDialogWithBuildIdAndCopyContentFreeReport_When_InputdebugQueryParam", async () => {
    mockSearch = "?inputdebug=1";
    const diag = createDiagnostics();
    const writeText = jest.fn().mockResolvedValue(undefined);
    setClipboard(writeText);
    const onCopied = jest.fn();
    render(<MinimalInputDiagnostics header={HEADER} buildId="index-ab12.js" diagnostics={diag} onCopied={onCopied} />);

    const dialog = screen.getByRole("dialog", { name: "Input diagnostics" });
    expect(dialog).toBeInTheDocument();
    expect(screen.getByTestId("input-diagnostics-header")).toHaveTextContent("index-ab12.js");

    act(() => {
      for (const [i, ch] of [..."qzxjk"].entries()) diag.pushInput("input", ch, i);
    });
    expect(screen.getAllByText("text(1)")).toHaveLength(5);

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy report" }));
    });
    const copied = writeText.mock.calls[0][0] as string;
    expect(copied.match(/text\(1\)/g)).toHaveLength(5);
    expect(copied).toContain("index-ab12.js");
    for (const ch of "qzxjk") expect(copied).not.toContain(`"${ch}"`);
    expect(onCopied).toHaveBeenCalledTimes(1);
  });

  it("panel_should_RenderNothingAndKeepBufferEmpty_When_NoInputdebugParam", () => {
    const diag = createDiagnostics();
    const { container } = render(<MinimalInputDiagnostics header={HEADER} buildId="b" diagnostics={diag} />);
    act(() => diag.pushInput("input", "a", 1));
    expect(container).toBeEmptyDOMElement();
    expect(diag.length).toBe(0);
  });

  it("panel_should_ShowSelectableReport_When_ClipboardBlocked", async () => {
    mockSearch = "?inputdebug=1";
    setClipboard(jest.fn().mockRejectedValue(new Error("denied")));
    const onCopied = jest.fn();
    render(<MinimalInputDiagnostics header={HEADER} buildId="b" diagnostics={createDiagnostics()} onCopied={onCopied} />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy report" }));
    });
    const box = screen.getByRole("textbox", { name: "Input diagnostics report" }) as HTMLTextAreaElement;
    expect(box.readOnly).toBe(true);
    expect(JSON.parse(box.value).header.renderer).toBe("xterm");
    expect(onCopied).not.toHaveBeenCalled();
  });

  it("panel_should_CloseAndEmptyBuffer_When_CloseButtonPressed", () => {
    mockSearch = "?inputdebug=1";
    const diag = createDiagnostics();
    render(<MinimalInputDiagnostics header={HEADER} buildId="b" diagnostics={diag} />);
    act(() => diag.pushInput("input", "a", 1));
    expect(diag.length).toBe(1);
    fireEvent.click(screen.getByRole("button", { name: "Close input diagnostics" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(diag.length).toBe(0);
    expect(diag.isOpen).toBe(false);
  });

  it("panel_should_HaveAccessibleNamesOnEveryButton_When_Rendered", () => {
    mockSearch = "?inputdebug=1";
    render(<MinimalInputDiagnostics header={HEADER} buildId="b" diagnostics={createDiagnostics()} />);
    for (const b of screen.getAllByRole("button")) expect(b).toHaveAccessibleName();
  });
});
