import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { OmnibarPresetList } from "./OmnibarPresetList";
import type { LauncherPresetEntry } from "@/lib/hooks/useLauncherPresets";

const mockPresets: LauncherPresetEntry[] = [
  { id: "codex-gpt5", label: "Codex GPT-5", argv: ["codex", "--model", "gpt-5"], program: "", defaultPath: "" },
  { id: "remote-claude", label: "Remote Claude", argv: ["ssh", "-t", "host"], program: "", defaultPath: "" },
];

describe("OmnibarPresetList", () => {
  it("OmnibarPresetList_should_RenderListboxWithOptionRows_When_PresetsProvided", () => {
    render(<OmnibarPresetList presets={mockPresets} loading={false} loadError={null} onSelect={jest.fn()} />);

    const listbox = screen.getByRole("listbox", { name: "Launcher presets" });
    const options = screen.getAllByRole("option");
    expect(listbox).toBeInTheDocument();
    expect(options).toHaveLength(2);
    expect(screen.getAllByTestId("preset-row")).toHaveLength(2);
  });

  it("OmnibarPresetList_should_ShowEmptyStatus_When_NoPresetsAndNoLoadError", () => {
    render(<OmnibarPresetList presets={[]} loading={false} loadError={null} onSelect={jest.fn()} />);

    const status = screen.getByTestId("preset-list-empty");
    expect(status).toHaveAttribute("role", "status");
    expect(status).toHaveTextContent(/No presets yet/);
  });

  it("OmnibarPresetList_should_ShowAlert_When_LoadErrorPresent", () => {
    render(
      <OmnibarPresetList presets={[]} loading={false} loadError='duplicate id "codex"' onSelect={jest.fn()} />
    );

    const alert = screen.getByTestId("preset-config-error");
    expect(alert).toHaveAttribute("role", "alert");
    expect(alert).toHaveTextContent('duplicate id "codex"');
  });

  it("OmnibarPresetList_should_ShowLoading_When_FirstFetchInFlight", () => {
    render(<OmnibarPresetList presets={[]} loading={true} loadError={null} onSelect={jest.fn()} />);

    expect(screen.getByTestId("preset-list-loading")).toBeInTheDocument();
  });

  it("OmnibarPresetList_should_KeepShowingList_When_BackgroundRefetchInFlight", () => {
    render(<OmnibarPresetList presets={mockPresets} loading={true} loadError={null} onSelect={jest.fn()} />);

    // A refetch (loading=true) with already-loaded presets must not flash the loading state.
    expect(screen.queryByTestId("preset-list-loading")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("preset-row")).toHaveLength(2);
  });

  it("OmnibarPresetList_should_CallOnSelect_When_RowClicked", async () => {
    const onSelect = jest.fn();
    render(<OmnibarPresetList presets={mockPresets} loading={false} loadError={null} onSelect={onSelect} />);

    await userEvent.click(screen.getAllByTestId("preset-row")[0]);
    expect(onSelect).toHaveBeenCalledWith(mockPresets[0]);
  });

  it("OmnibarPresetList_should_CallOnSelect_When_EnterPressedOnRow", async () => {
    const onSelect = jest.fn();
    render(<OmnibarPresetList presets={mockPresets} loading={false} loadError={null} onSelect={onSelect} />);

    const row = screen.getAllByTestId("preset-row")[1];
    row.focus();
    await userEvent.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith(mockPresets[1]);
  });

  it("OmnibarPresetList_should_CallOnSelect_When_SpacePressedOnRow", async () => {
    const onSelect = jest.fn();
    render(<OmnibarPresetList presets={mockPresets} loading={false} loadError={null} onSelect={onSelect} />);

    const row = screen.getAllByTestId("preset-row")[0];
    row.focus();
    await userEvent.keyboard(" ");
    expect(onSelect).toHaveBeenCalledWith(mockPresets[0]);
  });
});
