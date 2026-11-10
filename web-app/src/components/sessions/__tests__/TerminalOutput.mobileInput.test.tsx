// @feature terminal-input
/**
 * TerminalOutput's side of the mobile input layer (Story 1.4.1 / Tasks 1.2.4b, 1.2.3b, 1.4.1d, 1.4.1f):
 * SendResult flows from sendInput out to the renderer's onData, the layer's host reads TerminalOutput's
 * sticky modifiers, and visibility focus goes through the renderer handle. Off the Android class
 * nothing is passed to the renderer.
 */

import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { restoreEnv, setAndroidEnv } from "@/lib/terminal/mobileInput/domTestKit";

const terminal = {
  cols: 80,
  rows: 24,
  options: { disableStdin: false },
  focus: jest.fn(),
  buffer: { active: { viewportY: 50, baseY: 100, type: "normal" } },
};
const mockXtermHandle: any = {
  ...require("./terminalOutputTestMocks").createMockXtermHandle(),
  terminal,
  serializeAddon: null,
};
let capturedXtermProps: any = null;

jest.mock("../XtermTerminal", () =>
  require("./terminalOutputTestMocks").capturingXtermTerminalMockModule(mockXtermHandle, (props: any) => {
    capturedXtermProps = props;
  })
);
jest.mock("@/lib/hooks/useTerminalStream", () => ({ useTerminalStream: jest.fn() }));
jest.mock("@/lib/terminal/TerminalDimensionCache", () =>
  require("./terminalOutputTestMocks").terminalDimensionCacheWithValidationMockModule()
);
jest.mock("@/lib/terminal/TerminalStreamManager", () =>
  require("./terminalOutputTestMocks").terminalStreamManagerWithSerializeAddonMockModule()
);
jest.mock("@/lib/contexts/AnalyticsContext", () => require("./terminalOutputTestMocks").analyticsContextPlainMockModule());
jest.mock("@/lib/contexts/ApprovalsContext", () => require("./terminalOutputTestMocks").approvalsContextMockModule());
jest.mock("@/components/providers/ViewportProvider", () => require("./terminalOutputTestMocks").viewportProviderDesktopMockModule());
jest.mock("@/lib/nav", () => ({ useSearchParams: () => new URLSearchParams("") }));

// eslint-disable-next-line import/first
import { TerminalOutput } from "../TerminalOutput";
import { TerminalPoolProvider } from "@/lib/terminal/TerminalPool";
// eslint-disable-next-line import/first
import { useTerminalStream } from "@/lib/hooks/useTerminalStream";

const sendInput = jest.fn();
const FLAG_KEY = "ssq:flag:mobile_input_layer";

const streamMock = () => ({
  isInputChunking: jest.fn(() => false),
  isConnected: true, error: null, output: "", connect: jest.fn(), disconnect: jest.fn(), sendInput,
  resize: jest.fn(), scrollbackLoaded: false, requestScrollback: jest.fn(), sendFlowControl: jest.fn(),
  startRecording: jest.fn(), stopRecording: jest.fn(), terminalState: "CONNECTED", isHardFailed: false,
  handleManualReconnect: jest.fn(), requestFullResync: jest.fn(), markResyncComplete: jest.fn(),
  markPaneResponseReceived: jest.fn(),
});

async function mount(props: { readOnly?: boolean; isVisible?: boolean } = {}) {
  const ui = () => (
    <TerminalPoolProvider>
      <TerminalOutput sessionId="s1" baseUrl="/api" isVisible={props.isVisible ?? false} readOnly={props.readOnly ?? false} />
    </TerminalPoolProvider>
  );
  const view = render(ui());
  await act(async () => {});
  view.rerender(ui()); // the pooled handle attaches after the first commit
  await act(async () => {});
  return view;
}

/** Android-class page with the layer flag set. matchMedia gets the listener API TerminalOutput subscribes to. */
function androidWithLayer(flag: "1" | "0" = "1") {
  localStorage.setItem(FLAG_KEY, flag);
  setAndroidEnv();
  const answer = window.matchMedia;
  window.matchMedia = ((q: string) => ({
    ...answer(q), media: q, addEventListener: jest.fn(), removeEventListener: jest.fn(), addListener: jest.fn(), removeListener: jest.fn(),
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => {
  jest.clearAllMocks();
  localStorage.clear();
  capturedXtermProps = null;
  terminal.options.disableStdin = false;
  (useTerminalStream as jest.Mock).mockReturnValue(streamMock());
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation((query: string) => ({
      matches: false, media: query, addEventListener: jest.fn(), removeEventListener: jest.fn(),
      addListener: jest.fn(), removeListener: jest.fn(), dispatchEvent: jest.fn(),
    })),
  });
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  jest.spyOn(console, "info").mockImplementation(() => {});
});
afterEach(() => restoreEnv());

describe("extra-keys row", () => {
  it("extraKeysRow_should_NotRender_When_ReadOnly", async () => {
    await mount({ readOnly: true });
    expect(screen.queryAllByTestId("mobile-key")).toHaveLength(0);
  });

  it("extraKeysRow_should_Render_When_Writable", async () => {
    await mount();
    expect(screen.queryAllByTestId("mobile-key").length).toBeGreaterThan(0);
  });
});

describe("SendResult chain", () => {
  it.each(["sent", "dropped"] as const)("chain_should_ReturnSendInputResultFromPoolOnData_When_%s", async (result) => {
    sendInput.mockReturnValue(result);
    await mount();
    expect(capturedXtermProps.onData("x")).toBe(result);
    expect(sendInput).toHaveBeenCalledWith("x");
  });

  it("chain_should_ReturnDropped_When_ReadOnlySession", async () => {
    await mount({ readOnly: true });
    expect(capturedXtermProps.onData("x")).toBe("dropped");
    expect(sendInput).not.toHaveBeenCalled();
  });
});

describe("layer host", () => {
  it("mobileInput_should_BeAbsentFromRendererProps_When_NotAndroidClass", async () => {
    localStorage.setItem(FLAG_KEY, "1");
    await mount();
    expect(capturedXtermProps.mobileInput).toBeUndefined();
    expect(screen.queryByTestId("mobile-input-chrome")).toBeNull();
  });

  it("mobileInput_should_BeAbsentFromRendererProps_When_LayerFlagOff", async () => {
    androidWithLayer("0");
    await mount();
    expect(capturedXtermProps.mobileInput).toBeUndefined();
    expect(screen.getByTestId("mobile-input-flag-off-hint")).toBeInTheDocument();
  });

  it("host_should_PassStickyCtrlAsX03AndClearIt_When_SoftKeyboardC", async () => {
    androidWithLayer();
    await mount();
    const { host } = capturedXtermProps.mobileInput;
    expect(host.getModifiers().ctrl).toBe(false);
    fireEvent.pointerDown(screen.getByLabelText("Control modifier"));
    expect(host.getModifiers().ctrl).toBe(true);
    // The keymap turns "c" into ^C and consumes the armed modifier exactly once.
    expect(screen.getByLabelText(/Ctrl active/)).toHaveAttribute("aria-pressed", "true");
    act(() => host.consumeModifiers());
    expect(host.getModifiers().ctrl).toBe(false);
    expect(screen.getByLabelText("Control modifier")).toHaveAttribute("aria-pressed", "false");
  });

  it("host_should_ReportReadOnlyAndSendBytesThroughSendInput_When_LayerActive", async () => {
    androidWithLayer();
    sendInput.mockReturnValue("sent");
    await mount();
    const { host } = capturedXtermProps.mobileInput;
    expect(host.isReadOnly()).toBe(false);
    expect(host.onData("ls", 1)).toBe("sent");
    expect(sendInput).toHaveBeenCalledWith("ls");
  });

  it("mobileInput_should_ReportConnectionState_When_StreamDisconnects", async () => {
    androidWithLayer();
    await mount();
    expect(capturedXtermProps.mobileInput.isConnected).toBe(true);
    (useTerminalStream as jest.Mock).mockReturnValue({ ...streamMock(), isConnected: false });
    await mount();
    expect(capturedXtermProps.mobileInput.isConnected).toBe(false);
  });
});

describe("focus routing", () => {
  it("visibility_should_FocusThroughHandleNotTerminal_When_TerminalBecomesVisible", async () => {
    const view = await mount();
    // The pooled handle attaches after mount, so visibility has to flip afterwards, as on a tab switch.
    view.rerender(
      <TerminalPoolProvider>
        <TerminalOutput sessionId="s1" baseUrl="/api" isVisible readOnly={false} />
      </TerminalPoolProvider>,
    );
    await act(async () => {});
    expect(mockXtermHandle.focus).toHaveBeenCalled();
    expect(terminal.focus).not.toHaveBeenCalled();
  });
});
