import { act, fireEvent, render, screen } from "@testing-library/react";
import { restoreEnv, setAndroidEnv, setEnv } from "@/lib/terminal/mobileInput/domTestKit";
import { useMobileInputChrome, type MobileInputChrome, type UseMobileInputChromeOptions } from "./useMobileInputChrome";

let mockSearch = "";
jest.mock("@/lib/nav", () => ({ useSearchParams: () => new URLSearchParams(mockSearch) }));

const FLAG_KEY = "ssq:flag:mobile_input_layer";
let chrome: MobileInputChrome;

const SOURCE: UseMobileInputChromeOptions["source"] = {
  onData: () => "sent",
  getModifiers: () => ({ ctrl: false, alt: false, shift: false }),
  consumeModifiers: () => undefined,
};

function Harness(props: Partial<UseMobileInputChromeOptions>) {
  chrome = useMobileInputChrome({ streamState: "connected", readOnly: false, source: SOURCE, toolbarPx: 44, ...props });
  return <div data-testid="slot">{chrome.fragment}</div>;
}

const polite = () => screen.getByTestId("mobile-input-live-polite");
const assertive = () => screen.getByTestId("mobile-input-live-assertive");

beforeEach(() => {
  mockSearch = "";
  localStorage.clear();
  setAndroidEnv();
  jest.useFakeTimers();
  jest.spyOn(console, "info").mockImplementation(() => undefined);
});
afterEach(() => {
  jest.useRealTimers();
  restoreEnv();
  jest.restoreAllMocks();
});

describe("flag-off hint", () => {
  it("chrome_should_ShowFlagOffHint_When_AndroidClassAndFlagFalse", () => {
    localStorage.setItem(FLAG_KEY, "0");
    render(<Harness />);
    expect(screen.getByTestId("mobile-input-flag-off-hint")).toBeInTheDocument();
    expect(polite().textContent).toMatch(/Phone keyboard support is off/);
  });

  it("hint_should_UsePlainLanguageWithoutQueryOrFlagIdentifier_When_AndroidClassAndFlagFalse", () => {
    localStorage.setItem(FLAG_KEY, "0");
    render(<Harness />);
    const text = screen.getByTestId("mobile-input-flag-off-hint").textContent ?? "";
    expect(text).toContain("Phone keyboard support is off, so typing may not work well. You can turn it on in the feature flags panel.");
    expect(text).not.toContain("?");
    expect(text).not.toContain("mobile_input");
  });

  it("chrome_should_LogOneConsoleLine_When_FlagOffOnAndroidClass", () => {
    localStorage.setItem(FLAG_KEY, "0");
    render(<Harness />);
    expect(console.info).toHaveBeenCalledTimes(1);
  });

  it("hint_should_BeDismissible_When_DismissClicked", () => {
    localStorage.setItem(FLAG_KEY, "0");
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
    expect(screen.queryByTestId("mobile-input-flag-off-hint")).toBeNull();
  });

  it("chrome_should_RenderNothing_When_DesktopOrIosEvenWithFlagFalse", () => {
    localStorage.setItem(FLAG_KEY, "0");
    setEnv({ coarse: false, hover: true, ua: "Mozilla/5.0 (X11; Linux x86_64)" });
    const { container } = render(<Harness />);
    expect(screen.getByTestId("slot").childElementCount).toBe(0);
    expect(container.querySelector("[role=status]")).toBeNull();
    expect(console.info).not.toHaveBeenCalled();
    expect(chrome.fragment).toBeNull();
  });

  it("chrome_should_RenderNothingVisible_When_LayerFlagOn", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    expect(screen.queryByTestId("mobile-input-flag-off-hint")).toBeNull();
    expect(console.info).not.toHaveBeenCalled();
  });
});

describe("live regions", () => {
  it("chrome_should_RenderBothRegionsBeforeAnyMessage_When_Attachable", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    expect(polite()).toHaveAttribute("role", "status");
    expect(assertive()).toHaveAttribute("role", "alert");
    expect(polite().textContent).toBe("");
    expect(chrome.callbacks.describedBy).toBe(polite().id);
  });
});

describe("paste hint", () => {
  it("chrome_should_ShowAndAnnouncePasteFlattenedHintOnce_When_CallbackFires", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    act(() => chrome.callbacks.onPasteFlattened());
    expect(screen.getByTestId("mobile-input-paste-hint")).toBeInTheDocument();
    expect(polite().textContent).toMatch(/joined into one line/);
    const writes = jest.spyOn(polite(), "textContent", "set");
    act(() => chrome.callbacks.onPasteFlattened());
    expect(writes).not.toHaveBeenCalled();
  });

  it("chrome_should_RemovePasteHint_When_SixSecondsPass", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    act(() => chrome.callbacks.onPasteFlattened());
    act(() => jest.advanceTimersByTime(6000));
    expect(screen.queryByTestId("mobile-input-paste-hint")).toBeNull();
  });
});

describe("DeferredCommit indicator", () => {
  it("indicator_should_ShowCharactersWaitingAfter300msAndClearOnCommit_When_100GraphemeDictation", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    act(() => chrome.callbacks.onDeferredWaiting(100));
    const indicator = screen.getByTestId("mobile-input-deferred-indicator");
    expect(indicator.textContent).toContain("100 characters");
    expect(polite().textContent).toContain("100 characters");
    act(() => chrome.callbacks.onDeferredWaiting(null));
    expect(screen.queryByTestId("mobile-input-deferred-indicator")).toBeNull();
    expect(polite().textContent).toBe("");
  });

  it("indicator_should_NotCarryItsOwnRole_When_Rendered", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    act(() => chrome.callbacks.onDeferredWaiting(70));
    expect(screen.getByTestId("mobile-input-deferred-indicator")).not.toHaveAttribute("role");
  });
});

describe("read-only", () => {
  it("readOnlyTap_should_AnnounceOnPoliteRegionOncePerTenSeconds_When_SessionReadOnly", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness readOnly />);
    act(() => chrome.callbacks.onReadOnlyTap());
    expect(polite().textContent).toBe("Read-only session");
    act(() => chrome.callbacks.onPasteFlattened()); // another polite message replaces it
    act(() => chrome.callbacks.onReadOnlyTap());
    expect(polite().textContent).toMatch(/joined into one line/); // inside the 10 s window: not repeated
    act(() => jest.advanceTimersByTime(10_000));
    act(() => chrome.callbacks.onReadOnlyTap());
    expect(polite().textContent).toBe("Read-only session");
  });

  it("readOnlyTap_should_BeSilent_When_SessionNotReadOnly", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    act(() => chrome.callbacks.onReadOnlyTap());
    expect(polite().textContent).toBe("");
  });
});

describe("chrome budget", () => {
  const setViewportHeight = (height: number | null) =>
    Object.defineProperty(window, "visualViewport", {
      value: height === null ? null : { height, addEventListener: jest.fn(), removeEventListener: jest.fn() },
      configurable: true,
    });
  afterEach(() => setViewportHeight(null));

  it("chrome_should_ExposeCompactDensity_When_LandscapeKeyboardViewport", () => {
    localStorage.setItem(FLAG_KEY, "1");
    setViewportHeight(100);
    render(<Harness getCellHeightPx={() => 17} />);
    expect(chrome.budget?.density).toBe("compact");
    expect(screen.getByTestId("mobile-input-chrome")).toHaveAttribute("data-density", "compact");
  });

  it("chrome_should_BeComfortable_When_ViewportOrCellHeightUnknown", () => {
    localStorage.setItem(FLAG_KEY, "1");
    setViewportHeight(null);
    render(<Harness getCellHeightPx={() => 17} />);
    expect(chrome.budget?.density).toBe("comfortable");
  });
});

describe("host", () => {
  it("chrome_should_ProvideHostOnlyWhenLayerActive", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    expect(chrome.host).toBeDefined();
  });

  it("chrome_should_ProvideNoHost_When_FlagOffOrNotAttachable", () => {
    localStorage.setItem(FLAG_KEY, "0");
    render(<Harness />);
    expect(chrome.host).toBeUndefined();
  });

  it("host_should_ReadLatestSourceAndReadOnly_When_Rerendered", () => {
    localStorage.setItem(FLAG_KEY, "1");
    const first = jest.fn(() => "sent" as const);
    const second = jest.fn(() => "dropped" as const);
    const { rerender } = render(<Harness source={{ ...SOURCE, onData: first }} />);
    const host = chrome.host;
    if (!host) throw new Error("layer host missing");
    rerender(<Harness source={{ ...SOURCE, onData: second }} readOnly />);
    expect(host.onData("x" as never, 1 as never)).toBe("dropped");
    expect(first).not.toHaveBeenCalled();
    expect(host.isReadOnly()).toBe(true);
  });
});

describe("diagnostics panel", () => {
  it("chrome_should_MountDiagnosticsPanel_When_InputdebugRequestedOnAndroidClass", () => {
    localStorage.setItem(FLAG_KEY, "1");
    mockSearch = "?inputdebug=1";
    render(<Harness />);
    expect(screen.getByTestId("input-diagnostics")).toBeInTheDocument();
    expect(screen.getByTestId("input-diagnostics-header").textContent).toContain("xterm");
  });

  it("chrome_should_NotMountDiagnosticsPanel_When_NotRequested", () => {
    localStorage.setItem(FLAG_KEY, "1");
    render(<Harness />);
    expect(screen.queryByTestId("input-diagnostics")).toBeNull();
  });
});
