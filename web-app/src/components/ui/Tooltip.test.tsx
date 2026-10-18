import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Tooltip } from "./Tooltip";

jest.mock("./Tooltip.css", () => ({ tooltipContent: "c", tooltipArrow: "a", tooltipLink: "l" }));

async function hover(label: string) {
  await userEvent.hover(screen.getByRole("button", { name: label }));
}

describe("Tooltip", () => {
  it("renders no help link without helpSlug", async () => {
    render(<Tooltip label="Hint"><button>Trigger</button></Tooltip>);
    await hover("Trigger");
    expect((await screen.findAllByText("Hint")).length).toBeGreaterThan(0);
    expect(screen.queryByRole("link", { name: /learn more/i })).not.toBeInTheDocument();
  });

  it("links to /help?doc=<slug> when helpSlug is set", async () => {
    render(<Tooltip label="Hint" helpSlug="omnibar"><button>Trigger</button></Tooltip>);
    await hover("Trigger");
    const links = await screen.findAllByRole("link", { name: /learn more/i });
    links.forEach((l) => expect(l).toHaveAttribute("href", "/help?doc=omnibar"));
  });
});
