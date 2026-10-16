import { render, screen, fireEvent } from "@testing-library/react";
import { ProgramProbeSection } from "./ProgramProbeSection";
import {
  mockProbeCheck,
  probeStates,
  resetProbeMocks,
  setProbeHookState,
} from "@/lib/hooks/__mocks__/probeProgramMock";

jest.mock("@/lib/hooks/useProbeProgram", () => require("@/lib/hooks/__mocks__/probeProgramMock").hookMockFactory());

const OPTION = { value: "claude", label: "Claude Code", command: "claude" };

beforeEach(() => resetProbeMocks());

describe("ProgramProbeSection", () => {
  it("ProgramProbeSection_should_ShowResolvedPathBadge_When_ProgramFound", () => {
    setProbeHookState(probeStates.found);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getByText(/Found: \/usr\/bin\/claude/)).toBeInTheDocument();
  });

  it("ProgramProbeSection_should_ShowNotFoundBadgeWithoutDuplicateWarning_When_BinaryMissing", () => {
    setProbeHookState(probeStates.notFound);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getAllByTestId("preset-program-warning")).toHaveLength(1);
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.queryByText(/not found in PATH/)).toBeNull();
  });

  it("ProgramProbeSection_should_ShowSingleNotFoundMessage_When_ProgramMissing", () => {
    setProbeHookState(probeStates.notFound);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getAllByText(/Not found as an executable/)).toHaveLength(1);
  });

  it("ProgramProbeSection_should_ShowFlagsWithoutCheckPrompt_When_ServerReturnsFoundParsedForPreviouslyConfirmedProgram", () => {
    setProbeHookState(probeStates.found);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getByText("14 flags detected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Check/ })).toBeNull();
    expect(mockProbeCheck).toHaveBeenCalledWith();
  });

  it.each(["transportError", "busyOrError", "wrapper"])(
    "ProgramProbeSection_should_ShowNothing_When_TransportErrorOrWrapperOrBusy (%s)",
    (kind) => {
      setProbeHookState(probeStates[kind]);
      render(<ProgramProbeSection option={OPTION} />);
      expect(screen.queryByText(/flags detected/)).toBeNull();
      expect(screen.queryByTestId("preset-program-warning")).toBeNull();
    },
  );

  it("ProgramProbeSection_should_SendResolveOnlyOnSelectionAndConfirmOnlyOnCheck_When_SelectionChangesThenCheckClicked", () => {
    setProbeHookState(probeStates.needsConfirm);
    const { rerender } = render(<ProgramProbeSection option={OPTION} />);
    expect(mockProbeCheck).toHaveBeenLastCalledWith();
    rerender(<ProgramProbeSection option={{ ...OPTION, command: "aider" }} />);
    // The mocked hook returns a stable check fn, so the selection effect does not
    // refire here; the hook (not this component) resolves per command.
    fireEvent.click(screen.getByRole("button", { name: "Check" }));
    expect(mockProbeCheck).toHaveBeenLastCalledWith({ explicit: true });
    expect(mockProbeCheck).not.toHaveBeenCalledWith({ explicit: false });
  });

  it("ProgramProbeSection_should_DescribeSelectByStatusBadge_When_Rendered", () => {
    setProbeHookState(probeStates.found);
    render(<ProgramProbeSection option={OPTION} />);
    expect(screen.getByRole("status").id).toBe("omnibar-program-probe-status");
  });

  it("renders nothing when idle (no saved command)", () => {
    render(<ProgramProbeSection option={undefined} />);
    expect(screen.queryByRole("status")).toBeNull();
  });
  describe("saved cli_flags warning", () => {
    const foundWith = (names: string[]) =>
      ({
        ...probeStates.found,
        flags: names.map((name) => ({ name, short: "", takesValue: false, description: "", aliases: [] })),
      }) as never;
    const AIDER = { value: "aider", label: "Aider", command: "aider", cliFlags: "--yes-always --bogus" };

    it("ProgramProbeSection_should_ListOnlyBogus_When_SavedFlagsYesAlwaysAndBogus", () => {
      setProbeHookState(foundWith(["--yes-always"]), "aider");
      render(<ProgramProbeSection option={AIDER} />);
      const warning = screen.getByTestId("omnibar-flags-warning");
      expect(warning).toHaveTextContent("--bogus is not listed in aider --help.");
      expect(warning).not.toHaveTextContent("--yes-always");
      expect(warning).not.toHaveAttribute("role");
    });

    it.each(["notFound", "wrapper", "needsConfirm", "transportError", "noFlags", "checking"])(
      "ProgramProbeSection_should_ShowNoFlagWarning_When_%s",
      (kind) => {
        setProbeHookState(probeStates[kind], "aider");
        render(<ProgramProbeSection option={AIDER} />);
        expect(screen.queryByTestId("omnibar-flags-warning")).toBeNull();
      },
    );

    it("ProgramProbeSection_should_NotWarn_When_NoSavedFlags", () => {
      setProbeHookState(foundWith(["--yes-always"]), "aider");
      render(<ProgramProbeSection option={{ ...AIDER, cliFlags: undefined }} />);
      expect(screen.queryByTestId("omnibar-flags-warning")).toBeNull();
    });
  });
});

