import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { TerminalSettings } from "./TerminalSettings";
import { loadTerminalConfig, DEFAULT_TERMINAL_CONFIG } from "@/lib/config/terminalConfig";

beforeEach(() => localStorage.clear());
afterEach(() => localStorage.clear());

describe("TerminalSettings", () => {
  it("shows defaults when nothing is stored", () => {
    render(<TerminalSettings />);
    expect(screen.getByTestId("terminal-font-size")).toHaveValue(DEFAULT_TERMINAL_CONFIG.fontSize);
    expect(screen.getByTestId("terminal-cursor-block")).toHaveAttribute("aria-checked", "true");
  });

  it("persists a font size change and dispatches terminal-config-changed", () => {
    const onChange = jest.fn();
    window.addEventListener("terminal-config-changed", onChange);
    render(<TerminalSettings />);

    fireEvent.change(screen.getByTestId("terminal-font-size"), { target: { value: "18" } });

    expect(loadTerminalConfig().fontSize).toBe(18);
    expect(onChange).toHaveBeenCalled();
    window.removeEventListener("terminal-config-changed", onChange);
  });

  it("clamps font size to the supported range", () => {
    render(<TerminalSettings />);
    fireEvent.change(screen.getByTestId("terminal-font-size"), { target: { value: "99" } });
    expect(loadTerminalConfig().fontSize).toBe(32);
  });

  it("persists cursor style and blink, and reset restores defaults", () => {
    render(<TerminalSettings />);
    fireEvent.click(screen.getByTestId("terminal-cursor-bar"));
    fireEvent.click(screen.getByTestId("terminal-cursor-blink"));
    expect(loadTerminalConfig()).toMatchObject({ cursorStyle: "bar", cursorBlink: false });

    fireEvent.click(screen.getByTestId("terminal-settings-reset"));
    expect(loadTerminalConfig()).toMatchObject({
      cursorStyle: DEFAULT_TERMINAL_CONFIG.cursorStyle,
      cursorBlink: DEFAULT_TERMINAL_CONFIG.cursorBlink,
    });
  });
});
