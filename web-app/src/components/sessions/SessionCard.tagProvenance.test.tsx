/**
 * Story 6.2.1 (ux.md Surface 1/2): provenance tooltip on rule-derived tag
 * pills, and Unclassified's distinct no-tooltip-on-manual-tags contrast.
 */

import { render, screen } from "@testing-library/react";
import { SessionCard } from "./SessionCard";
import type { Session } from "@/gen/session/v1/types_pb";

jest.mock("@connectrpc/connect", () => require("./__tests__/sessionCardTestFixtures").mockConnect());
jest.mock("@connectrpc/connect-web", () => require("./__tests__/sessionCardTestFixtures").mockConnectWeb());
jest.mock("@/lib/contexts/ReviewQueueContext", () => require("./__tests__/sessionCardTestFixtures").mockReviewQueueContext());
jest.mock("@/lib/contexts/SessionServiceContext", () => require("./__tests__/sessionCardTestFixtures").mockSessionServiceContext());
jest.mock("@/lib/store", () => require("./__tests__/sessionCardTestFixtures").mockStore());
jest.mock("@/lib/store/sessionsSlice", () => require("./__tests__/sessionCardTestFixtures").mockSessionsSlice());
jest.mock("@/lib/hooks/useTerminalSnapshot", () => require("./__tests__/sessionCardTestFixtures").mockUseTerminalSnapshot());
jest.mock("@/lib/hooks/useFocusTrap", () => require("./__tests__/sessionCardTestFixtures").mockUseFocusTrap());
jest.mock("@/components/ui/AppLink", () => require("./__tests__/sessionCardTestFixtures").mockAppLink());
jest.mock("@/components/ui/Modal", () => require("./__tests__/sessionCardTestFixtures").mockModal());
jest.mock("@/components/ui/Tooltip", () => require("./__tests__/sessionCardTestFixtures").mockTooltip());
jest.mock("@/lib/hooks/useSessionActions", () => require("./__tests__/sessionCardTestFixtures").mockUseSessionActions());

jest.mock("@/lib/hooks/useTaggingRuleNames", () => ({
  useTaggingRuleNames: () => ({ "seed-bugfix": "Bugfix branch" }),
}));

const baseSession: Partial<Session> = {
  id: "s1",
  title: "Test Session",
  status: 1 as Session["status"],
  category: "",
  path: "/tmp/session",
  branch: "",
  program: "claude",
};

describe("SessionCard — tag provenance (ux.md Surface 1)", () => {
  it("SessionCard_should_ShowRuleProvenanceTooltip_When_TagHasRuleProvenanceEntry", () => {
    const session = {
      ...baseSession,
      tags: ["Bugfix", "MyTag"],
      ruleTagProvenance: { Bugfix: "seed-bugfix" },
    } as unknown as Session;
    render(<SessionCard session={session} onUpdateTags={jest.fn()} />);

    const bugfixPill = screen.getByText("Bugfix");
    expect(bugfixPill.getAttribute("title")).toBe("Applied by rule: Bugfix branch");
    expect(bugfixPill.getAttribute("aria-label")).toBe("Tag: Bugfix (auto-applied by rule)");

    const manualPill = screen.getByText("MyTag");
    expect(manualPill.getAttribute("title")).toBeNull();
    expect(manualPill.getAttribute("aria-label")).toBe("Tag: MyTag");
  });

  it("SessionCard_should_ShowUnclassifiedTooltipFramingItAsTransient_When_UnclassifiedTagPresent", () => {
    const session = {
      ...baseSession,
      tags: ["Unclassified"],
      ruleTagProvenance: {},
    } as unknown as Session;
    render(<SessionCard session={session} onUpdateTags={jest.fn()} />);

    const pill = screen.getByText(/Unclassified/);
    expect(pill.getAttribute("title")).toBe(
      "LLM classification failed or timed out — will retry next poll cycle"
    );
  });

  it("SessionCard_should_MakeTagPillsFocusable_When_Rendered", () => {
    const session = { ...baseSession, tags: ["MyTag"], ruleTagProvenance: {} } as unknown as Session;
    render(<SessionCard session={session} onUpdateTags={jest.fn()} />);

    expect(screen.getByText("MyTag").getAttribute("tabIndex")).toBe("0");
  });
});
