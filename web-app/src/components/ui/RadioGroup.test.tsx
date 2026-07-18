/**
 * Tests for the shared RadioGroup component (extracted from
 * OmnibarCreationPanel.tsx's SessionTypeRadioGroup — see project_plans/
 * backlog-configurable-pipeline/implementation/plan.md Epic 3.1).
 *
 * Covers:
 *  1. ArrowRight cycles to the next option, calling onChange and updating aria-checked
 *  2. ArrowLeft cycles to the previous option (wraps around)
 *  3. Tab alone never mutates selection (roving tabindex only reacts to arrow keys)
 *  4. Accessible name resolves via aria-labelledby (not a duplicated aria-label string)
 *  5. Accessible description resolves via aria-describedby when a hint is provided
 *  6. No accessible description when hintForValue returns undefined
 *  7. Roving tabindex: only the checked option has tabIndex=0, all others -1
 *  8. Clicking an option calls onChange with that option's value
 */

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RadioGroup } from "./RadioGroup";

// The jest styleMock for `.css.ts` files wraps every export (including plain
// `style()` string exports, not just `recipe()` calls) in a callable proxy
// function, which triggers a benign "Invalid value for prop className" React
// warning under test. This is a pre-existing jest/vanilla-extract mock
// limitation (see web-app/src/__mocks__/styleMock.js), not a real bug — other
// component test suites in this repo (e.g. OmnibarCreationPanel.attach.test.tsx)
// silence it the same way.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

type Val = "a" | "b" | "c";

const OPTIONS: { value: Val; label: string; description?: string }[] = [
  { value: "a", label: "A", description: "Option A description" },
  { value: "b", label: "B", description: "Option B description" },
  { value: "c", label: "C", description: "Option C description" },
];

function renderGroup(overrides: Partial<React.ComponentProps<typeof RadioGroup<Val>>> = {}) {
  const onChange = jest.fn();
  const utils = render(
    <RadioGroup
      options={OPTIONS}
      value="a"
      onChange={onChange}
      groupLabel="Test"
      {...overrides}
    />
  );
  return { onChange, ...utils };
}

describe("RadioGroup keyboard navigation", () => {
  it("RadioGroup_should_CallOnChangeWithNextOption_When_ArrowRightPressed", async () => {
    const user = userEvent.setup();
    const { onChange } = renderGroup();

    const optionA = screen.getByRole("radio", { name: "A" });
    optionA.focus();
    await user.keyboard("{ArrowRight}");

    expect(onChange).toHaveBeenCalledWith("b");
  });

  it("updates aria-checked on the newly selected option when ArrowRight is pressed", () => {
    const onChange = jest.fn();
    const { rerender } = render(
      <RadioGroup options={OPTIONS} value="a" onChange={onChange} groupLabel="Test" />
    );

    const optionB = screen.getByRole("radio", { name: "B" });
    expect(optionB).toHaveAttribute("aria-checked", "false");

    // Simulate the controlled-component update triggered by onChange("b")
    rerender(<RadioGroup options={OPTIONS} value="b" onChange={onChange} groupLabel="Test" />);

    expect(screen.getByRole("radio", { name: "B" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "A" })).toHaveAttribute("aria-checked", "false");
  });

  it("RadioGroup_should_CallOnChangeWithPreviousOption_When_ArrowLeftPressed_AndWrapsAround", async () => {
    const user = userEvent.setup();
    const onChange = jest.fn();
    render(<RadioGroup options={OPTIONS} value="a" onChange={onChange} groupLabel="Test" />);

    const optionA = screen.getByRole("radio", { name: "A" });
    optionA.focus();
    await user.keyboard("{ArrowLeft}");

    // Wraps from the first option to the last
    expect(onChange).toHaveBeenCalledWith("c");
  });

  it("RadioGroup_should_NotChangeSelection_When_TabPressedWithoutArrowKeys", async () => {
    const user = userEvent.setup();
    const { onChange } = renderGroup();

    const optionA = screen.getByRole("radio", { name: "A" });
    optionA.focus();
    await user.keyboard("{Tab}");

    expect(onChange).not.toHaveBeenCalled();
  });

  it("calls onChange with the clicked option's value", async () => {
    const user = userEvent.setup();
    const { onChange } = renderGroup();

    await user.click(screen.getByRole("radio", { name: "C" }));

    expect(onChange).toHaveBeenCalledWith("c");
  });
});

describe("RadioGroup accessibility wiring (Story 3.1.2)", () => {
  it("RadioGroup_should_ExposeAccessibleNameViaAriaLabelledby_When_GroupLabelProvided", () => {
    renderGroup({ groupLabel: "Pipeline mode" });

    const group = screen.getByRole("radiogroup", { name: "Pipeline mode" });
    expect(group).toHaveAttribute("aria-labelledby");
    expect(group).not.toHaveAttribute("aria-label");
  });

  it("RadioGroup_should_ExposeAccessibleDescriptionViaAriaDescribedby_When_HintTextProvided", () => {
    renderGroup({
      groupLabel: "Pipeline mode",
      hintForValue: () => "Choose which skills drive this item",
    });

    const group = screen.getByRole("radiogroup", {
      name: "Pipeline mode",
      description: /Choose which skills/,
    });
    expect(group).toHaveAttribute("aria-describedby");
  });

  it("does not set aria-describedby when hintForValue is omitted", () => {
    renderGroup({ groupLabel: "Pipeline mode" });

    const group = screen.getByRole("radiogroup", { name: "Pipeline mode" });
    expect(group).not.toHaveAttribute("aria-describedby");
  });
});

describe("RadioGroup roving tabindex", () => {
  it("RadioGroup_should_OnlySetTabIndexZeroOnCheckedOption_When_Rendered", () => {
    renderGroup({ value: "b" });

    expect(screen.getByRole("radio", { name: "A" })).toHaveAttribute("tabIndex", "-1");
    expect(screen.getByRole("radio", { name: "B" })).toHaveAttribute("tabIndex", "0");
    expect(screen.getByRole("radio", { name: "C" })).toHaveAttribute("tabIndex", "-1");
  });

  it("falls back to tabIndex=0 on the first option when no option matches the current value", () => {
    renderGroup({ value: "unmatched" as Val });

    expect(screen.getByRole("radio", { name: "A" })).toHaveAttribute("tabIndex", "0");
    expect(screen.getByRole("radio", { name: "B" })).toHaveAttribute("tabIndex", "-1");
    expect(screen.getByRole("radio", { name: "C" })).toHaveAttribute("tabIndex", "-1");
  });
});
