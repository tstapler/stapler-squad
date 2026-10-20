/**
 * Story 1.3.2 banner states (design/ux.md section 2): copy per mode, per-class
 * honesty, fail-closed Unknown/Stale, the nested live region, the review line.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import { fireEvent } from "@testing-library/react";
import { EngineModeBanner } from "./EngineModeBanner";
import { bannerText, formReminder, saveConfirmation } from "@/lib/hooks/useEngineModeStatus";
import type { ClassMode, EngineModeView } from "@/lib/hooks/useEngineModeStatus";

const BN_OPERATOR_ROW = "flag-row-backlog-workflow-engine-enforce";

function bnView(overrides: Partial<EngineModeView> = {}): EngineModeView {
  return {
    kind: "shadow",
    classes: { operator: "shadow", agent: "shadow", system: "shadow" },
    divergence24h: 3,
    stale: false,
    lastCheckedAt: Date.now(),
    fallbackActive: false,
    invalidatorWired: true,
    review: null,
    flagRowId: BN_OPERATOR_ROW,
    ...overrides,
  };
}

function bnPartial(operator: ClassMode, agent: ClassMode, system: ClassMode): EngineModeView {
  return bnView({
    kind: "partial",
    classes: { operator, agent, system },
    flagRowId: "flag-row-backlog-workflow-engine-enforce-agent",
  });
}

const BN_ENFORCED = bnView({
  kind: "enforced",
  classes: { operator: "enforce", agent: "enforce", system: "enforce" },
});

describe("EngineModeBanner", () => {
  it("Shadow: reads the spec sentence, has no role itself and one empty status region", () => {
    render(<EngineModeBanner view={bnView()} onRetry={jest.fn()} />);
    const banner = screen.getByTestId("engine-mode-banner");
    expect(banner).not.toHaveAttribute("role");
    expect(banner).toHaveTextContent(
      "Shadow: edits are compared against the built-in rules but are not enforced. 3 differences in the last 24h."
    );
    const live = screen.getByTestId("engine-mode-banner-live");
    expect(live).toHaveAttribute("role", "status");
    expect(live).toBeEmptyDOMElement();
    expect(banner.querySelectorAll('[role="status"]')).toHaveLength(1);
  });

  it("Shadow with zero differences", () => {
    expect(bannerText(bnView({ divergence24h: 0 }))).toContain("No differences in the last 24h.");
    expect(bannerText(bnView({ divergence24h: 1 }))).toContain("1 difference in the last 24h.");
    expect(bannerText(bnView({ divergence24h: null }))).toContain("Difference count unavailable.");
  });

  it("Partly enforced: states each class's own mode and never the plain Enforced wording", () => {
    render(<EngineModeBanner view={bnPartial("enforce", "shadow", "shadow")} onRetry={jest.fn()} />);
    const text = screen.getByTestId("engine-mode-banner").textContent ?? "";
    expect(text).toContain(
      "Partly enforced. Operators (this UI): enforced. Agents: shadow, not enforced. Automation: shadow, not enforced."
    );
    expect(text).not.toContain("Enforced: edits here");
  });

  it("Partly enforced reports an off class as off", () => {
    expect(bannerText(bnPartial("enforce", "off", "shadow"))).toContain("Agents: off, not enforced. Automation: shadow, not enforced.");
  });

  it("Enforced only when all three classes are enforced", () => {
    render(<EngineModeBanner view={BN_ENFORCED} onRetry={jest.fn()} />);
    expect(screen.getByTestId("engine-mode-banner")).toHaveTextContent(/^.?\s*Enforced: edits here change/);
  });

  it("Off", () => {
    expect(bannerText(bnView({ kind: "off" }))).toBe(
      "Not in use: the built-in 9-stage workflow is active. Edits are saved but have no effect."
    );
  });

  it("Loading: aria-busy, never claims enforcement, no link or retry", () => {
    render(<EngineModeBanner view={bnView({ kind: "loading" })} onRetry={jest.fn()} />);
    const banner = screen.getByTestId("engine-mode-banner");
    expect(banner).toHaveAttribute("aria-busy", "true");
    expect(banner).toHaveTextContent("Checking whether these settings are enforced...");
    expect(screen.queryByTestId("engine-mode-banner-retry")).not.toBeInTheDocument();
  });

  it("Unknown: fail-closed copy with a working Retry", () => {
    const onRetry = jest.fn();
    render(<EngineModeBanner view={bnView({ kind: "unknown" })} onRetry={onRetry} />);
    expect(screen.getByTestId("engine-mode-banner")).toHaveTextContent(
      "Couldn't determine whether these settings are enforced. Treat edits as not applied."
    );
    expect(screen.getByTestId("engine-mode-banner").textContent).not.toMatch(/Enforced:/);
    fireEvent.click(screen.getByTestId("engine-mode-banner-retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("Stale: keeps the last value, adds the last-checked note and Retry, never promotes to Enforced", () => {
    render(
      <EngineModeBanner view={bnView({ stale: true, lastCheckedAt: Date.now() - 5 * 60_000 })} onRetry={jest.fn()} />
    );
    const text = screen.getByTestId("engine-mode-banner").textContent ?? "";
    expect(text).toContain("Shadow: edits are compared");
    expect(text).toContain("Last checked 5m ago; couldn't refresh.");
    expect(text).not.toContain("Enforced: edits here");
    expect(screen.getByTestId("engine-mode-banner-retry")).toBeInTheDocument();
  });

  it("links to the first class not yet enforced", () => {
    const { rerender } = render(<EngineModeBanner view={bnPartial("enforce", "shadow", "shadow")} onRetry={jest.fn()} />);
    const link = screen.getByRole("link", { name: "Change in Feature Flags" });
    expect(link).toHaveAttribute("href", "/settings/features#flag-row-backlog-workflow-engine-enforce-agent");
    rerender(<EngineModeBanner view={BN_ENFORCED} onRetry={jest.fn()} />);
    expect(screen.getByRole("link", { name: "Change in Feature Flags" })).toHaveAttribute(
      "href",
      `/settings/features#${BN_OPERATOR_ROW}`
    );
  });

  it("announces changes after the first resolved value, not the initial render, and not unchanged refetches", () => {
    const { rerender } = render(<EngineModeBanner view={bnView({ kind: "loading" })} onRetry={jest.fn()} />);
    const live = screen.getByTestId("engine-mode-banner-live");
    expect(live).toBeEmptyDOMElement();
    rerender(<EngineModeBanner view={bnView()} onRetry={jest.fn()} />);
    expect(live).toBeEmptyDOMElement();
    // Same mode text from a new fetch (new object): still silent.
    rerender(<EngineModeBanner view={bnView({ lastCheckedAt: Date.now() + 1 })} onRetry={jest.fn()} />);
    expect(live).toBeEmptyDOMElement();
    rerender(<EngineModeBanner view={bnView({ kind: "off" })} onRetry={jest.fn()} />);
    expect(live).toHaveTextContent("Not in use: the built-in 9-stage workflow is active.");
  });

  it("announces entering the Unknown state after a resolved value", () => {
    const { rerender } = render(<EngineModeBanner view={bnView()} onRetry={jest.fn()} />);
    rerender(<EngineModeBanner view={bnView({ kind: "unknown" })} onRetry={jest.fn()} />);
    expect(screen.getByTestId("engine-mode-banner-live")).toHaveTextContent("Couldn't determine whether");
  });

  describe("review line (UX-55)", () => {
    it.each([
      [{ state: "overdue", date: "2026-11-01" }, /^Review overdue: the soak review date \(2026-11-01\) has passed and no caller class is enforced\. Review the soak evidence in docs\/how-to\/flip-workflow-engine-enforcement\.md\.$/],
      [{ state: "not_due", date: "2026-11-01" }, /^Soak review due 2026-11-01\.$/],
      [{ state: "enforced", date: "2026-11-01" }, /^Soak review date 2026-11-01\.$/],
    ] as const)("renders %j", (review, expected) => {
      render(<EngineModeBanner view={bnView({ review: { ...review } })} onRetry={jest.fn()} />);
      expect(screen.getByTestId("engine-mode-review").textContent).toMatch(expected);
    });

    it("is absent without review data", () => {
      render(<EngineModeBanner view={bnView()} onRetry={jest.fn()} />);
      expect(screen.queryByTestId("engine-mode-review")).not.toBeInTheDocument();
    });
  });

  it("an engine fallback is called out whenever the mode claims enforcement", () => {
    expect(bannerText({ ...BN_ENFORCED, fallbackActive: true })).toContain("fell back to the built-in rules");
    expect(bannerText(bnView({ fallbackActive: true }))).not.toContain("fell back");
  });
});

describe("formReminder and saveConfirmation (UX-41, UX-52)", () => {
  const cases: Array<[string, EngineModeView, string, string]> = [
    ["enforced", BN_ENFORCED, "Mode: Enforced.", "Saved and enforced"],
    [
      "partly enforced",
      bnPartial("enforce", "shadow", "shadow"),
      "Mode: Partly enforced. Saved edits apply to operators only; agents and automation are in shadow.",
      "Saved. Enforced for operators only; not enforced for agents and automation.",
    ],
    ["shadow", bnView(), "Mode: Shadow. Saved edits are not enforced yet.", "Saved. Not enforced yet (shadow)"],
    ["off", bnView({ kind: "off" }), "Mode: Off. Edits are saved but have no effect.", "Saved. Not in use (off)"],
    ["loading", bnView({ kind: "loading" }), "Mode: checking...", "Saved. Couldn't confirm whether it is enforced."],
    [
      "unknown",
      bnView({ kind: "unknown" }),
      "Mode: couldn't be determined. Treat saved edits as not applied.",
      "Saved. Couldn't confirm whether it is enforced.",
    ],
  ];

  it.each(cases)("%s", (_name, view, reminder, confirmation) => {
    expect(formReminder(view)).toBe(reminder);
    expect(saveConfirmation(view)).toBe(confirmation);
  });

  it("stale: never Enforced-only wording, even when the last value was all-enforced", () => {
    const stale = { ...BN_ENFORCED, stale: true, lastCheckedAt: Date.now() - 2 * 60_000 };
    expect(formReminder(stale)).toBe("Mode: Enforced (last checked 2m ago; couldn't refresh). Saved edits may not be enforced.");
    expect(saveConfirmation(stale)).toBe("Saved. Couldn't confirm whether it is enforced.");
  });

  it("adds the unrefreshed-engine note when the invalidator is not wired", () => {
    expect(saveConfirmation(bnView({ invalidatorWired: false }))).toBe(
      "Saved. Not enforced yet (shadow) Saved, but the running engine was not refreshed."
    );
  });

  it("never renders an empty reminder for any kind", () => {
    for (const [, view] of cases) expect(formReminder(view).length).toBeGreaterThan(0);
  });
});
