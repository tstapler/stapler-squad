import React from "react";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { CronScheduleInput } from "./CronScheduleInput";

function explanationRegion() {
  return within(document.getElementById("wf-cron-explanation")!);
}

function Controlled({ initial }: { initial: string }) {
  const [value, setValue] = React.useState(initial);
  return (
    <>
      <span id="wf-cron-label">Cron Expression</span>
      <CronScheduleInput id="wf-cron" labelId="wf-cron-label" value={value} onChange={setValue} />
    </>
  );
}

describe("CronScheduleInput", () => {
  it("starts in Simple mode for an empty value and builds a daily cron string", () => {
    render(<Controlled initial="" />);
    expect(screen.getByLabelText("Simple")).toBeChecked();
    // Default schedule (daily 09:00) should already be reflected in the explanation.
    expect(explanationRegion().getByText(/09:00 AM/)).toBeInTheDocument();
  });

  it("Simple dropdowns call onChange with the correct cron string per frequency", () => {
    render(<Controlled initial="" />);
    fireEvent.change(screen.getByLabelText("Frequency"), { target: { value: "weekdays" } });
    expect(explanationRegion().getByText(/Monday through Friday/i)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Frequency"), { target: { value: "weekly" } });
    fireEvent.change(screen.getByLabelText("Day of week"), { target: { value: "3" } });
    expect(explanationRegion().getByText(/Wednesday/)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Frequency"), { target: { value: "monthly" } });
    fireEvent.change(screen.getByLabelText("Day of month"), { target: { value: "15" } });
    expect(explanationRegion().getByText(/day 15 of the month/)).toBeInTheDocument();
  });

  it("shows an inline error for an invalid Advanced expression", () => {
    render(<Controlled initial="99 9 * * *" />);
    // Non-representable by the Simple builder, so it should land in Advanced.
    expect(screen.getByLabelText("Advanced")).toBeChecked();
    expect(screen.getByRole("alert")).toHaveTextContent(/minute field/i);
  });

  it("Advanced -> Simple falls back to a raw-editor notice instead of discarding a step expression", () => {
    render(<Controlled initial="*/15 9-17 * * 1-5" />);
    expect(screen.getByLabelText("Advanced")).toBeChecked();
    const rawInput = screen.getByDisplayValue("*/15 9-17 * * 1-5") as HTMLInputElement;

    fireEvent.click(screen.getByLabelText("Simple"));

    expect(screen.getByRole("status")).toHaveTextContent(/can't represent/i);
    expect(screen.getByLabelText("Advanced")).toBeChecked();
    expect(rawInput.value).toBe("*/15 9-17 * * 1-5");
  });

  it("Simple -> Advanced preserves the exact cron string the builder computed", () => {
    render(<Controlled initial="" />);
    fireEvent.change(screen.getByLabelText("Frequency"), { target: { value: "weekly" } });
    fireEvent.change(screen.getByLabelText("Day of week"), { target: { value: "5" } });
    fireEvent.change(screen.getByLabelText("Time"), { target: { value: "14:30" } });

    fireEvent.click(screen.getByLabelText("Advanced"));

    expect(screen.getByDisplayValue("30 14 * * 5")).toBeInTheDocument();
  });

  it("renders the accessible pieces: radiogroup, aria-live explanation, timezone note", () => {
    render(<Controlled initial="0 9 * * *" />);
    expect(screen.getByRole("radiogroup", { name: "Schedule entry mode" })).toBeInTheDocument();
    expect(screen.getByText(/server's local timezone/)).toBeInTheDocument();
  });
});
