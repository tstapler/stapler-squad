import React from "react";
import { render, screen } from "@testing-library/react";
import { DiagnoseResultCard, parseDiagnosisNote } from "./DiagnoseResultCard";

describe("DiagnoseResultCard", () => {
  it("renders outcome badge and summary for a diagnosis note", () => {
    render(<DiagnoseResultCard message={"Diagnosis result: BUG_FILED\n\nWake loop found."} />);
    expect(screen.getByTestId("diagnose-result-card")).toBeInTheDocument();
    expect(screen.getByText("BUG_FILED")).toBeInTheDocument();
    expect(screen.getByText("Wake loop found.")).toBeInTheDocument();
  });

  it("renders an outcome-only note without a summary", () => {
    render(<DiagnoseResultCard message="Diagnosis result: NO_ACTION" />);
    expect(screen.getByText("NO_ACTION")).toBeInTheDocument();
  });

  it("falls back to plain text for non-diagnosis notes", () => {
    render(<DiagnoseResultCard message="checked in on this" />);
    expect(screen.queryByTestId("diagnose-result-card")).not.toBeInTheDocument();
    expect(screen.getByText("checked in on this")).toBeInTheDocument();
  });

  it("falls back to plain text when the outcome token is malformed", () => {
    const msg = "Diagnosis result: not an outcome\n\nbody";
    expect(parseDiagnosisNote(msg)).toBeNull();
    render(<DiagnoseResultCard message={msg} />);
    expect(screen.queryByTestId("diagnose-result-card")).not.toBeInTheDocument();
  });
});
