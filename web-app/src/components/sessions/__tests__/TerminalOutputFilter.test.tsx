import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { TerminalOutputFilter } from "../TerminalOutputFilter";

const LINES = ["build started", "ERROR: disk full", "all good"];

describe("TerminalOutputFilter", () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());

  it("shows a display-only hint and no results before a query is typed", () => {
    render(<TerminalOutputFilter getLines={() => LINES} onClose={() => {}} />);
    expect(screen.getByTestId("terminal-filter-status")).toHaveTextContent(/display only/i);
    expect(screen.queryByTestId("terminal-filter-results")).toBeNull();
  });

  it("lists matching lines with their numbers", () => {
    render(<TerminalOutputFilter getLines={() => LINES} onClose={() => {}} />);
    fireEvent.change(screen.getByTestId("terminal-filter-input"), { target: { value: "error" } });

    const results = screen.getByTestId("terminal-filter-results");
    expect(results).toHaveTextContent("2");
    expect(results).toHaveTextContent("ERROR: disk full");
    expect(screen.getByTestId("terminal-filter-status")).toHaveTextContent("1 matching line");
  });

  it("picks up new output on the refresh interval", () => {
    let lines = [...LINES];
    render(<TerminalOutputFilter getLines={() => lines} onClose={() => {}} />);
    fireEvent.change(screen.getByTestId("terminal-filter-input"), { target: { value: "error" } });
    expect(screen.getByTestId("terminal-filter-status")).toHaveTextContent("1 matching line");

    lines = [...lines, "ERROR: again"];
    act(() => { jest.advanceTimersByTime(1000); });
    expect(screen.getByTestId("terminal-filter-status")).toHaveTextContent("2 matching lines");
  });

  it("reports an invalid regex instead of crashing", () => {
    render(<TerminalOutputFilter getLines={() => LINES} onClose={() => {}} />);
    fireEvent.click(screen.getByTestId("terminal-filter-regex"));
    fireEvent.change(screen.getByTestId("terminal-filter-input"), { target: { value: "(" } });
    expect(screen.getByTestId("terminal-filter-status")).toHaveTextContent(/invalid pattern/i);
  });

  it("closes on Escape and on the close button", () => {
    const onClose = jest.fn();
    render(<TerminalOutputFilter getLines={() => LINES} onClose={onClose} />);
    fireEvent.keyDown(screen.getByTestId("terminal-filter-input"), { key: "Escape" });
    fireEvent.click(screen.getByTestId("terminal-filter-close"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
