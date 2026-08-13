import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TerminalContextMenu } from "../TerminalContextMenu";

beforeEach(() => {
  jest.spyOn(console, "log").mockImplementation(() => {});
  jest.spyOn(console, "warn").mockImplementation(() => {});
  Object.defineProperty(navigator, "clipboard", {
    value: {
      writeText: jest.fn().mockResolvedValue(undefined),
      readText: jest.fn().mockResolvedValue(""),
    },
    configurable: true,
    writable: true,
  });
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("TerminalContextMenu", () => {
  const defaultProps = {
    x: 100,
    y: 200,
    hasSelection: true,
    onCopy: jest.fn(),
    onSelectAll: jest.fn(),
    onDismiss: jest.fn(),
  };

  it("renders menu items in document.body portal", () => {
    render(<TerminalContextMenu {...defaultProps} />);
    const menu = document.body.querySelector('[data-testid="terminal-context-menu"]');
    expect(menu).not.toBeNull();
  });

  it("positions menu at provided x/y coordinates", () => {
    render(<TerminalContextMenu {...defaultProps} x={150} y={250} />);
    const menu = document.body.querySelector('[data-testid="terminal-context-menu"]') as HTMLElement;
    expect(menu.style.left).toBe("150px");
    expect(menu.style.top).toBe("250px");
  });

  it("Copy button is enabled when hasSelection is true", () => {
    render(<TerminalContextMenu {...defaultProps} hasSelection={true} />);
    const copyBtn = screen.getByText("Copy");
    expect(copyBtn).not.toBeDisabled();
  });

  it("Copy button is disabled when hasSelection is false", () => {
    render(<TerminalContextMenu {...defaultProps} hasSelection={false} />);
    const copyBtn = screen.getByText("Copy");
    expect(copyBtn).toBeDisabled();
  });

  it("calls onCopy and onDismiss when Copy is clicked", () => {
    const onCopy = jest.fn();
    const onDismiss = jest.fn();
    render(
      <TerminalContextMenu
        {...defaultProps}
        hasSelection={true}
        onCopy={onCopy}
        onDismiss={onDismiss}
      />
    );
    fireEvent.click(screen.getByText("Copy"));
    expect(onCopy).toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalled();
  });

  it("calls onSelectAll and onDismiss when Select All is clicked", () => {
    const onSelectAll = jest.fn();
    const onDismiss = jest.fn();
    render(
      <TerminalContextMenu
        {...defaultProps}
        onSelectAll={onSelectAll}
        onDismiss={onDismiss}
      />
    );
    fireEvent.click(screen.getByText("Select All"));
    expect(onSelectAll).toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalled();
  });

  it("calls onDismiss when Escape key is pressed", () => {
    const onDismiss = jest.fn();
    render(<TerminalContextMenu {...defaultProps} onDismiss={onDismiss} />);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onDismiss).toHaveBeenCalled();
  });

  it("calls onDismiss when clicking outside the menu", () => {
    const onDismiss = jest.fn();
    render(<TerminalContextMenu {...defaultProps} onDismiss={onDismiss} />);
    fireEvent.mouseDown(document.body);
    expect(onDismiss).toHaveBeenCalled();
  });

  it("does not show Paste when onPaste prop is not provided", () => {
    render(<TerminalContextMenu {...defaultProps} onPaste={undefined} />);
    expect(screen.queryByText("Paste")).toBeNull();
  });

  it("always renders Select All", () => {
    render(<TerminalContextMenu {...defaultProps} />);
    expect(screen.getByText("Select All")).toBeInTheDocument();
  });
});
