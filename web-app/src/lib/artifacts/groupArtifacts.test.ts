import { create } from "@bufbuild/protobuf";
import { timestampFromMs } from "@bufbuild/protobuf/wkt";
import { ArtifactRecordProtoSchema, SessionArtifactsSchema, type ArtifactRecordProto, type SessionArtifacts } from "@/gen/session/v1/types_pb";
import { groupArtifacts, isRedacted } from "./groupArtifacts";

const T = (hh: number, mm: number) => Date.UTC(2026, 10, 5, hh, mm);
type Case = ArtifactRecordProto["record"];

function rec(at: number | undefined, record: Case): ArtifactRecordProto {
  return create(ArtifactRecordProtoSchema, { kind: record.case ?? "", firstSeenAt: at === undefined ? undefined : timestampFromMs(at), record });
}
const pr = (at: number, number: number, source = "created") => rec(at, { case: "pr", value: { owner: "acme", repo: "app", number, source, host: "", branch: "", base: "", $typeName: "session.v1.ArtifactPRRecord" } });
const commit = (at: number | undefined, sha: string, subject = "", branch = "") => rec(at, { case: "commit", value: { sha, subject, branch, $typeName: "session.v1.ArtifactCommitRecord" } });
const plan = (at: number, path: string) => rec(at, { case: "planDoc", value: { path, $typeName: "session.v1.ArtifactPlanDocRecord" } });
const artifacts = (records: ArtifactRecordProto[]): SessionArtifacts => create(SessionArtifactsSchema, { records });

describe("groupArtifacts", () => {
  it("groupArtifacts_should_putEachItemUnderTheLatestPrCreatedAtOrBeforeIt_AndTheRestUnderUnassigned", () => {
    const g = groupArtifacts(artifacts([
      commit(T(9, 0), "aaaaaaa1111"),
      pr(T(10, 0), 11),
      commit(T(10, 30), "1a2b3c4d5e6f"),
      pr(T(11, 0), 12),
      plan(T(11, 15), "project_plans/x/plan.md"),
    ]));
    expect(g.prGroups.map((x) => [x.number, x.items.map((i) => i.title)])).toEqual([
      [11, ["1a2b3c4"]],
      [12, ["project_plans/x/plan.md"]],
    ]);
    expect(g.unassigned.map((i) => i.title)).toEqual(["aaaaaaa"]);
  });

  it("groupArtifacts_should_sendItemsWithoutATimestampToUnassigned", () => {
    const g = groupArtifacts(artifacts([pr(T(10, 0), 11), commit(undefined, "deadbeef0000")]));
    expect(g.prGroups[0].items).toHaveLength(0);
    expect(g.unassigned).toHaveLength(1);
  });

  it("groupArtifacts_should_listMentionedPrsSeparately_AndNotDuplicateACreatedOne", () => {
    const g = groupArtifacts(artifacts([pr(T(10, 0), 11), pr(T(10, 5), 11, "mentioned"), pr(T(10, 6), 40, "mentioned"), pr(T(10, 7), 40, "mentioned")]));
    expect(g.prGroups.map((x) => x.number)).toEqual([11]);
    expect(g.mentioned.map((x) => x.number)).toEqual([40]);
  });

  it("groupArtifacts_should_countRecordsItCannotShow", () => {
    const bad = create(ArtifactRecordProtoSchema, { kind: "commit" });
    expect(groupArtifacts(artifacts([bad, commit(T(1, 0), "abcdef0123")])).skipped).toBe(1);
  });

  it("isRedacted_should_flagValuesWithTheRedactionMarker", () => {
    expect(isRedacted("https://ci.example/x?token=[redacted]")).toBe(true);
    expect(isRedacted("https://ci.example/x")).toBe(false);
  });
});
