import { readCorpusFiles } from "./fixtureCorpus";
import { lintFixtureFile, scanSecrets } from "./fixtureSchema";

const synthetic = {
  id: "x",
  source: "synthetic",
  keyboard: "gboard",
  notes: "https://example.com/source",
  events: [{ kind: "tick", t: 0 }],
  expectedBytes: "",
  finalText: "",
};
const lint = (obj: object, name = "case.json") => lintFixtureFile(name, JSON.stringify(obj));

describe("fixture lint", () => {
  it("lint_should_PassCleanSyntheticFixture_When_AllFieldsPresent", () => {
    expect(lint(synthetic)).toEqual([]);
  });

  it("lint_should_FailNamingFile_When_DeviceFixtureNotScrubbedOrNoSignoff", () => {
    const device = { ...synthetic, source: "device" };
    expect(lint(device, "dev-a.json")).toEqual([
      'dev-a.json: source "device" requires scrubbed: true',
      'dev-a.json: source "device" requires a non-empty signoff',
    ]);
    expect(lint({ ...device, scrubbed: true }, "dev-b.json")).toEqual([
      'dev-b.json: source "device" requires a non-empty signoff',
    ]);
    expect(lint({ ...device, scrubbed: true, signoff: "  " }, "dev-c.json")).toHaveLength(1);
    expect(lint({ ...device, signoff: "owner 2026-11-08" }, "dev-d.json")).toEqual([
      'dev-d.json: source "device" requires scrubbed: true',
    ]);
    expect(lint({ ...device, scrubbed: "true", signoff: "owner" }, "dev-e.json")).toHaveLength(1);
  });

  it("lint_should_PassDeviceFixture_When_ScrubbedAndSignedOff", () => {
    expect(lint({ ...synthetic, source: "device", scrubbed: true, signoff: "tstapler 2026-11-08" })).toEqual([]);
  });

  it("lint_should_FailSecretScan_When_SyntheticFixtureContainsGhpPrefix", () => {
    const token = "ghp" + "_" + "abcdefghijklmnop";
    const problems = lint({ ...synthetic, expectedBytes: token, finalText: token }, "leaky.json");
    expect(problems).toEqual(["leaky.json: secret-shaped string (GitHub token)"]);
  });

  it("lint_should_FailNamingFile_When_SourceMissingOrFileNotJson", () => {
    const { source: _omit, ...noSource } = synthetic;
    expect(lint(noSource, "nosrc.json")).toContain('nosrc.json: field "source" must be "synthetic" or "device"');
    expect(lintFixtureFile("bad.json", "{oops")).toEqual(["bad.json: not valid JSON"]);
  });

  it.each([
    ["aws", "AKIA" + "ABCDEFGHIJKLMNOP", "AWS access key"],
    ["slack", "xoxb-" + "123456789012-abcdef", "Slack token"],
    ["pem", "-----BEGIN RSA " + "PRIVATE KEY-----", "private key block"],
    ["url", "https://user:" + "hunter2@host/", "credential in URL"],
    ["assignment", "password=" + "correcthorse", "password assignment"],
  ])("scanSecrets_should_Flag_%s", (_n, text, name) => {
    expect(scanSecrets(`typed ${text} here`)).toContain(name);
  });

  it("scanSecrets_should_StayQuiet_When_OrdinaryTypedText", () => {
    expect(scanSecrets("ls -la; git status; the quick brown fox")).toEqual([]);
  });

  it("lint_should_PassEveryCommittedFixture_When_GlobbedOverCorpusDir", () => {
    const files = readCorpusFiles();
    expect(files.length).toBeGreaterThan(0);
    const problems = files.flatMap((f) => (f.name.endsWith(".json") ? lintFixtureFile(f.name, f.text) : [`${f.name}: non-JSON file in corpus`]));
    expect(problems).toEqual([]);
  });
});
