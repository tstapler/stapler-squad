// T-E2-38: the decision records the notification work rests on exist, carry a status, and say
// what later code review leans on (ADR-001 one choke point, ADR-006 overlay).
import fs from "fs";
import path from "path";

const PLAN = path.join(process.cwd(), "..", "project_plans", "notification-tray-and-hidden-session-gate");
const DECISIONS = path.join(PLAN, "decisions");

function adrFiles(): string[] {
  return fs
    .readdirSync(DECISIONS)
    .filter((f) => /^ADR-\d{3}-.*\.md$/.test(f))
    .sort();
}

describe("notification tray and hidden-session gate ADRs", () => {
  it("adr_index_should_list_001_through_010_with_a_status_each", () => {
    const files = adrFiles();
    expect(files.map((f) => f.slice(0, 7))).toEqual(
      Array.from({ length: 10 }, (_, i) => `ADR-${String(i + 1).padStart(3, "0")}`),
    );
    for (const file of files) {
      const text = fs.readFileSync(path.join(DECISIONS, file), "utf8");
      expect(text).toMatch(/^\*\*Status\*\*: \S+/m);
    }

    // The plan's ADR index names every record, in order.
    const plan = fs.readFileSync(path.join(PLAN, "implementation", "plan.md"), "utf8");
    const index = plan.split("\n").find((line) => line.startsWith("**ADRs**:")) ?? "";
    let from = 0;
    for (let n = 1; n <= 10; n++) {
      const at = index.indexOf(`ADR-${String(n).padStart(3, "0")}`, from);
      expect(at).toBeGreaterThanOrEqual(from);
      from = at;
    }
  });

  it("adr_006_and_001_should_state_overlay_and_single_choke_point_when_read", () => {
    const find = (prefix: string) => fs.readFileSync(path.join(DECISIONS, adrFiles().find((f) => f.startsWith(prefix))!), "utf8");

    const overlay = find("ADR-006");
    expect(overlay.split("\n")[0]).toMatch(/Non-Modal.*Layout-Neutral Overlay/);
    expect(overlay).toMatch(/overlay-only by default/);

    const gate = find("ADR-001");
    expect(gate.split("\n")[0]).toMatch(/One Delivery Gate/);
    expect(gate).toMatch(/EventBus Publish Filter/);
    expect(gate).toMatch(/choke point/);
  });
});
