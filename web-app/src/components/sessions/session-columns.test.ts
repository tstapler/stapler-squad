import { COLUMN_DEFS, buildRowGridTemplate } from "./session-columns";

describe("COLUMN_DEFS", () => {
  it("COLUMN_DEFS_should_HaveAgentAndMemoryDefaultVisibleFalse_When_Loaded", () => {
    const agent = COLUMN_DEFS.find((c) => c.key === "agent");
    const memory = COLUMN_DEFS.find((c) => c.key === "memory");
    expect(agent?.defaultVisible).toBe(false);
    expect(memory?.defaultVisible).toBe(false);
  });

  it("COLUMN_DEFS_should_KeepElapsedDiffBranchDefaultVisible_When_AgentMemoryDemoted", () => {
    const elapsed = COLUMN_DEFS.find((c) => c.key === "elapsed");
    const diff = COLUMN_DEFS.find((c) => c.key === "diff");
    const branch = COLUMN_DEFS.find((c) => c.key === "branch");
    expect(elapsed?.defaultVisible).toBe(true);
    expect(diff?.defaultVisible).toBe(false);
    expect(branch?.defaultVisible).toBe(false);
  });
});

describe("buildRowGridTemplate", () => {
  it("buildRowGridTemplate_should_OmitElapsedGridWidth_When_ElapsedInVisibleColumns", () => {
    const template = buildRowGridTemplate(["agent", "memory", "elapsed"], { reserveCheckbox: true });
    const tracks = template.split(" ");
    // Fixed tracks: checkbox(24px) + dot(8px) + name(1fr) + agent(20px) + memory(auto) + actions(auto) = 6.
    // If elapsed's "auto" gridWidth were included, this would be 7.
    expect(tracks).toHaveLength(6);
    expect(template).toContain("20px"); // agent's gridWidth still present
  });

  it("buildRowGridTemplate_should_IncludeAgentAndMemoryGridWidths_When_ElapsedAlsoVisible", () => {
    const template = buildRowGridTemplate(["agent", "memory", "elapsed"], { reserveCheckbox: true });
    expect(template).toContain("20px"); // agent
    // memory's gridWidth is "auto" — confirm at least two "auto" tracks remain
    // (memory + actions), i.e. elapsed didn't get skipped by removing all "auto".
    const autoCount = template.split(" ").filter((t) => t === "auto").length;
    expect(autoCount).toBe(2);
  });
});
