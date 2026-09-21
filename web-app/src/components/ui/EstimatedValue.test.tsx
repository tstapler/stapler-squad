import React from "react";
import { render, screen } from "@testing-library/react";
import { EstimatedValue } from "./EstimatedValue";

describe("EstimatedValue", () => {
  it("EstimatedValue_should_renderTildePrefixedValue_when_childrenProvided", () => {
    render(<EstimatedValue title="Modeled estimate">$5.00</EstimatedValue>);
    expect(screen.getByText("~$5.00")).toBeInTheDocument();
  });

  it("EstimatedValue_should_setTitleAttribute_when_titlePropProvided", () => {
    render(<EstimatedValue title="Tool-type-level attribution">$5.00</EstimatedValue>);
    expect(screen.getByTitle("Tool-type-level attribution")).toBeInTheDocument();
  });

  it("EstimatedValue_should_linkAriaDescribedbyToTooltipText_when_rendered", () => {
    render(<EstimatedValue title="Modeled estimate">$5.00</EstimatedValue>);
    const marker = screen.getByText("~$5.00");
    const describedById = marker.getAttribute("aria-describedby");
    expect(describedById).toBeTruthy();
    const description = document.getElementById(describedById as string);
    expect(description).toHaveTextContent("Modeled estimate");
  });
});
