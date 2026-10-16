/**
 * Tests for InputModeSetting component.
 *
 * Covers:
 *  1. Renders all three options (auto/desktop/touch)
 *  2. Defaults to "auto" selected when no localStorage value is set
 *  3. Marks the persisted option as checked on mount
 *  4. Clicking an option calls setInputModeOverride and persists to localStorage
 *  5. Only one option is aria-checked at a time
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { InputModeSetting } from "./InputModeSetting";
import { INPUT_MODE_OVERRIDE_KEY } from "@/lib/hooks/useInputModeOverride";

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  localStorage.clear();
});

describe("InputModeSetting", () => {
  it("renders all three options", () => {
    render(<InputModeSetting />);
    expect(screen.getByTestId("input-mode-option-auto")).toBeInTheDocument();
    expect(screen.getByTestId("input-mode-option-desktop")).toBeInTheDocument();
    expect(screen.getByTestId("input-mode-option-touch")).toBeInTheDocument();
  });

  it("defaults to auto selected when no localStorage value is set", () => {
    render(<InputModeSetting />);
    expect(screen.getByTestId("input-mode-option-auto")).toHaveAttribute("aria-checked", "true");
    expect(screen.getByTestId("input-mode-option-desktop")).toHaveAttribute("aria-checked", "false");
    expect(screen.getByTestId("input-mode-option-touch")).toHaveAttribute("aria-checked", "false");
  });

  it("marks the persisted option as checked on mount", () => {
    localStorage.setItem(INPUT_MODE_OVERRIDE_KEY, "desktop");
    render(<InputModeSetting />);
    expect(screen.getByTestId("input-mode-option-desktop")).toHaveAttribute("aria-checked", "true");
    expect(screen.getByTestId("input-mode-option-auto")).toHaveAttribute("aria-checked", "false");
  });

  it("clicking an option marks it checked and persists to localStorage", () => {
    render(<InputModeSetting />);
    fireEvent.click(screen.getByTestId("input-mode-option-touch"));
    expect(screen.getByTestId("input-mode-option-touch")).toHaveAttribute("aria-checked", "true");
    expect(localStorage.getItem(INPUT_MODE_OVERRIDE_KEY)).toBe("touch");
  });

  it("only one option is aria-checked at a time", () => {
    render(<InputModeSetting />);
    fireEvent.click(screen.getByTestId("input-mode-option-desktop"));
    const checked = ["auto", "desktop", "touch"].filter(
      (v) => screen.getByTestId(`input-mode-option-${v}`).getAttribute("aria-checked") === "true"
    );
    expect(checked).toEqual(["desktop"]);
  });
});
