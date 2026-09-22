import React, { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { FlagCombobox } from "./FlagCombobox";
import type { FlagOption } from "@/lib/flags/flagTokens";

const FLAGS: FlagOption[] = [
  { name: "--model", short: "-m", takesValue: true, description: "Model to use", aliases: [] },
  { name: "--verbose", short: "-v", takesValue: false, description: "Chatty output", aliases: [] },
  { name: "--model-settings", short: "", takesValue: true, description: "Settings file", aliases: [] },
];

function Harness({ initial, flags, onBlur }: { initial: string; flags: FlagOption[]; onBlur?: () => void }) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <FlagCombobox id="f" value={value} onChange={setValue} flags={flags} testId="f-input" describedBy="ext" />
      <button onBlur={onBlur}>next</button>
    </>
  );
}

const input = () => screen.getByTestId("f-input") as HTMLInputElement;

/** Types into the input with the caret at the end, as a user would. */
function type(text: string) {
  const el = input();
  el.focus();
  fireEvent.change(el, { target: { value: text, selectionStart: text.length, selectionEnd: text.length } });
}

describe("FlagCombobox", () => {
  it("FlagCombobox_should_SetAriaAttributesAndActiveDescendant_When_DownPressed", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(input()).toHaveAttribute("role", "combobox");
    expect(input()).toHaveAttribute("aria-autocomplete", "list");
    expect(input()).toHaveAttribute("aria-expanded", "true");
    const listbox = screen.getByRole("listbox");
    expect(input()).toHaveAttribute("aria-controls", listbox.id);
    expect(input()).toHaveAttribute("aria-activedescendant", screen.getAllByRole("option")[0].id);
  });

  it("FlagCombobox_should_ExposeComboboxListboxOptionAttributes_When_Open", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    expect(input()).not.toHaveAttribute("aria-activedescendant");
    const options = screen.getAllByRole("option");
    expect(options.map((o) => o.id)).toEqual(["f-option-0", "f-option-1"]);
    expect(options.every((o) => o.getAttribute("aria-selected") === "false")).toBe(true);
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(options[0]).toHaveAttribute("aria-selected", "true");
    expect(options.every((o) => o.querySelector("button, a, input") === null)).toBe(true);
  });

  it("FlagCombobox_should_InsertModelAndShowInlineDescription_When_DownThenEnter", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--yes --mo");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(screen.getByText("Model to use")).toBeInTheDocument();
    expect(input().getAttribute("aria-describedby")).toContain("f-active-description");
    expect(input().getAttribute("aria-describedby")).toContain("ext");
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(input().value).toBe("--yes --model ");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("FlagCombobox_should_InsertModelWithTwoKeystrokesOrOneTap_When_MoTyped", () => {
    const { unmount } = render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(input().value).toBe("--model ");
    unmount();
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    fireEvent.mouseDown(screen.getAllByRole("option")[0]);
    expect(input().value).toBe("--model ");
  });

  it("FlagCombobox_should_ExposeAccessibleNameWithTakesValueAndDescription_When_OptionRendered", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--v");
    expect(screen.getByRole("option", { name: "--verbose" })).toBeInTheDocument();
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(input()).toHaveAccessibleDescription(/Chatty output/);
    fireEvent.keyDown(input(), { key: "Escape" });
    type("--mo");
    expect(screen.getByRole("option", { name: "--model, takes a value" })).toBeInTheDocument();
  });

  it("FlagCombobox_should_WrapWhenArrowingPastEnds_When_ListOpen", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    fireEvent.keyDown(input(), { key: "ArrowUp" });
    expect(input()).toHaveAttribute("aria-activedescendant", "f-option-1");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(input()).toHaveAttribute("aria-activedescendant", "f-option-0");
  });

  it("FlagCombobox_should_NotAcceptOnTabAndKeepText_When_ListOpenAndNoArrowNavigation", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--v");
    const tab = fireEvent.keyDown(input(), { key: "Tab" });
    expect(tab).toBe(true); // not default-prevented, so focus moves normally
    expect(input().value).toBe("--v");
    expect(screen.queryByRole("listbox")).toBeNull();

    type("--v");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    const accepted = fireEvent.keyDown(input(), { key: "Tab" });
    expect(accepted).toBe(false);
    expect(input().value).toBe("--verbose ");
  });

  it("FlagCombobox_should_RenderPlainInputNoListbox_When_FlagsEmpty", () => {
    render(<Harness initial="" flags={[]} />);
    type("--mo");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input()).not.toHaveAttribute("role");
    for (const attr of ["aria-expanded", "aria-controls", "aria-autocomplete", "aria-activedescendant", "aria-invalid"]) {
      expect(input()).not.toHaveAttribute(attr);
    }
    expect(input()).toHaveAttribute("autocapitalize", "off");
    expect(input()).toHaveAttribute("spellcheck", "false");
  });

  it("FlagCombobox_should_KeepSameInputNodeAndFocus_When_FlagsArriveLate", () => {
    const { rerender } = render(<FlagCombobox id="f" value="--mo" onChange={() => {}} flags={[]} testId="f-input" />);
    const before = input();
    before.focus();
    before.setSelectionRange(2, 2);
    rerender(<FlagCombobox id="f" value="--mo" onChange={() => {}} flags={FLAGS} testId="f-input" />);
    expect(input()).toBe(before);
    expect(document.activeElement).toBe(before);
    expect(before.selectionStart).toBe(2);
    expect(before).toHaveAttribute("role", "combobox");
  });

  it("FlagCombobox_should_CloseWithoutClearing_When_EscapePressed", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    fireEvent.keyDown(input(), { key: "Escape" });
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input().value).toBe("--mo");
    expect(input()).toHaveFocus();
  });

  it("FlagCombobox_should_NeverPreventDefaultOnTypingAndKeepText_When_Escape", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    for (const key of ["m", "-", " "]) {
      expect(fireEvent.keyDown(input(), { key })).toBe(true);
    }
    expect(fireEvent.keyDown(input(), { key: "Enter" })).toBe(true); // no active option: Enter passes through
    expect(input().value).toBe("--mo");
  });

  it("FlagCombobox_should_ShowActiveOptionDescriptionInline_When_ArrowNavigation", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    expect(screen.queryByText("Model to use")).toBeNull();
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(screen.getByText("Model to use").closest("[role=option]")).toBe(screen.getAllByRole("option")[0]);
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(screen.queryByText("Model to use")).toBeNull();
    expect(screen.getByText("Settings file")).toBeInTheDocument();
  });

  it("FlagCombobox_should_SelectOnMouseDownWithoutBlurReprobe_When_OptionTapped", () => {
    const onBlur = jest.fn();
    render(<Harness initial="" flags={FLAGS} onBlur={onBlur} />);
    type("--mo");
    const blur = jest.fn();
    input().addEventListener("blur", blur);
    const notPrevented = fireEvent.mouseDown(screen.getAllByRole("option")[1]);
    expect(notPrevented).toBe(false); // preventDefault keeps focus in the input
    expect(input().value).toBe("--model-settings ");
    expect(blur).not.toHaveBeenCalled();
    expect(input()).toHaveFocus();
  });

  it("FlagCombobox_should_CloseList_When_InputBlurs", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("--mo");
    fireEvent.blur(input());
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input().value).toBe("--mo");
  });

  it("FlagCombobox_should_NotOpen_When_TokenNotDash", () => {
    render(<Harness initial="" flags={FLAGS} />);
    type("mo");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input()).toHaveAttribute("aria-expanded", "false");
  });
});
