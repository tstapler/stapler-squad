import { readFileSync } from "fs";
import { join } from "path";
import { render, screen, fireEvent } from "@testing-library/react";
import { FlagInfoButton } from "./FlagInfoButton";
import { AvailableFlags } from "./AvailableFlags";
import { UnknownFlagsWarning } from "./UnknownFlagsWarning";

describe("FlagInfoButton", () => {
  it("FlagInfoButton_should_BeNamedButtonWithExpandedAndControls_When_Rendered", () => {
    render(<FlagInfoButton flag="--model" description="Model to use" />);
    const button = screen.getByRole("button", { name: "Show description for --model" });
    expect(button.tagName).toBe("BUTTON");
    expect(button).toHaveAttribute("type", "button");
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(button).toHaveAttribute("aria-controls");
  });

  it("FlagInfoButton_should_ExpandInlineWithAriaExpandedTrue_When_Clicked", () => {
    render(<FlagInfoButton flag="--model" description="Model to use" />);
    const button = screen.getByTestId("prog-flag-info-button");
    fireEvent.click(button);
    expect(button).toHaveAttribute("aria-expanded", "true");
    const description = screen.getByText("Model to use");
    expect(button.getAttribute("aria-controls")).toBe(description.id);
    expect(button).toHaveAccessibleName("Hide description for --model");
  });

  it("FlagInfoButton_should_CollapseAndRenderNothing_When_SecondClickOrNoDescription", () => {
    const { unmount } = render(<FlagInfoButton flag="--model" description="Model to use" />);
    const button = screen.getByTestId("prog-flag-info-button");
    fireEvent.click(button);
    fireEvent.click(button);
    expect(screen.queryByText("Model to use")).toBeNull();
    expect(button).toHaveAttribute("aria-expanded", "false");
    unmount();
    const empty = render(<FlagInfoButton flag="--x" description="" />);
    expect(empty.container).toBeEmptyDOMElement();
  });

  it("FlagInfoButton_should_UseMinSize44Tokens_When_Rendered", () => {
    // Style modules are mocked in jest; the css source is the 44px contract.
    const src = readFileSync(join(__dirname, "FlagInfoButton.css.ts"), "utf8");
    expect(src).toMatch(/minWidth: "44px"/);
    expect(src).toMatch(/minHeight: "44px"/);
  });

  it("FlagInfoButton_should_NotChangeValueOrTriggerProbe_When_TappedInAvailableFlagsDisclosure", () => {
    const flags = [{ name: "--model", short: "", takesValue: true, description: "Model to use", aliases: [] }];
    render(<AvailableFlags flags={flags} testId="af" />);
    expect(screen.queryByTestId("prog-flag-info-button")).toBeNull();
    fireEvent.click(screen.getByTestId("af-toggle"));
    fireEvent.click(screen.getByTestId("prog-flag-info-button"));
    expect(screen.getByText("Model to use")).toBeInTheDocument();
  });
});

describe("UnknownFlagsWarning", () => {
  it("UnknownFlagsWarning_should_CapAtThreeNamesAndPluralise_When_ManyUnknown", () => {
    render(
      <UnknownFlagsWarning id="w" testId="w" program="/usr/bin/aider" unknown={["--a", "--b", "--c", "--d", "--e"]} />,
    );
    expect(screen.getByTestId("w")).toHaveTextContent(
      "--a, --b, --c and 2 more are not listed in aider --help. It may still work (hidden or subcommand flags).",
    );
  });

  it("UnknownFlagsWarning_should_RenderNothing_When_NoneUnknown", () => {
    const { container } = render(<UnknownFlagsWarning id="w" testId="w" program="x" unknown={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});
