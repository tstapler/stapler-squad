import React from "react";
import { render, screen } from "@testing-library/react";
import { SourceSection } from "./SourceSection";

const base = { externalUrl: "https://github.com/o/r/issues/1", externalId: "1", labels: [], defaultExpanded: true };

describe("SourceSection working host", () => {
  it("shows the claiming host when set", () => {
    render(<SourceSection {...base} claimingHostId="host_01K" />);
    expect(screen.getByTestId("source-working-host")).toHaveTextContent("Working host: host_01K");
  });

  it("shows nothing when there is no claiming host", () => {
    render(<SourceSection {...base} />);
    expect(screen.queryByTestId("source-working-host")).toBeNull();
  });
});
