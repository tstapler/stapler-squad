import { formatRespawnReason } from "./formatRespawnReason";

describe("formatRespawnReason", () => {
  it("formatRespawnReason_should_ReturnHumanLabel_When_ReasonIsAutonomousTurnRespawn", () => {
    expect(formatRespawnReason("autonomous_turn_respawn")).toBe("Autonomous turn respawn");
  });

  it("formatRespawnReason_should_ReturnHumanLabel_When_ReasonIsStaleWorkRemediation", () => {
    expect(formatRespawnReason("stale_work_remediation")).toBe("Stale work session remediation");
  });

  it("formatRespawnReason_should_ReturnHumanLabel_When_ReasonIsReviewRespawn", () => {
    expect(formatRespawnReason("review_respawn")).toBe("Abandoned review respawn");
  });

  it("formatRespawnReason_should_ReturnHumanLabel_When_ReasonIsTriageRespawn", () => {
    expect(formatRespawnReason("triage_respawn")).toBe("Orphaned triage respawn");
  });

  it("formatRespawnReason_should_ReturnRawReason_When_ReasonIsUnrecognized", () => {
    expect(formatRespawnReason("some_future_reason")).toBe("some_future_reason");
  });
});
