import { WorkflowDetector, WorkflowEntry } from "./WorkflowDetector";
import { InputType } from "../types";

const WORKFLOWS: WorkflowEntry[] = [
  {
    slug: "daily-standup",
    name: "Daily Standup",
    description: "Run daily standup workflow",
    targetDirectory: "/home/user/project",
    sessionType: "directory",
    inputTemplate: "Run standup for {{input}}",
  },
  {
    slug: "fix-bug",
    name: "Fix Bug",
    inputTemplate: "Fix bug: {{input}}",
  },
];

function makeDetector(workflows = WORKFLOWS): WorkflowDetector {
  return new WorkflowDetector(workflows);
}

describe("WorkflowDetector", () => {
  describe("basic detection", () => {
    it("WorkflowDetector_should_returnNull_When_inputLacksAtPrefix", () => {
      // T-UNIT-TS-101
      const d = makeDetector();
      expect(d.detect("daily-standup")).toBeNull();
      expect(d.detect("some regular text")).toBeNull();
      expect(d.detect("/some/path")).toBeNull();
    });

    it("WorkflowDetector_should_matchWorkflow_When_inputStartsWithAt", () => {
      // T-UNIT-TS-102
      const d = makeDetector();
      const result = d.detect("@daily-standup");
      expect(result).not.toBeNull();
      expect(result!.type).toBe(InputType.Workflow);
    });

    it("WorkflowDetector_should_returnConfidence1_When_slugKnown", () => {
      // T-UNIT-TS-103
      const d = makeDetector();
      const result = d.detect("@daily-standup");
      expect(result!.confidence).toBe(1.0);
      expect(result!.metadata!.workflowFound).toBe(true);
    });

    it("WorkflowDetector_should_returnConfidence04_When_slugUnknown", () => {
      // T-UNIT-TS-104
      const d = makeDetector();
      const result = d.detect("@unknown-workflow");
      expect(result).not.toBeNull();
      expect(result!.type).toBe(InputType.Workflow);
      expect(result!.confidence).toBe(0.4);
      expect(result!.metadata!.workflowFound).toBe(false);
    });

    it("WorkflowDetector_should_extractArg_When_inputContainsArgAfterSlug", () => {
      // T-UNIT-TS-105
      const d = makeDetector();
      const result = d.detect("@daily-standup team-alpha");
      expect(result!.metadata!.workflowArg).toBe("team-alpha");
    });

    it("WorkflowDetector_should_emptyArg_When_noArgSupplied", () => {
      // T-UNIT-TS-106
      const d = makeDetector();
      const result = d.detect("@daily-standup");
      expect(result!.metadata!.workflowArg).toBe("");
    });
  });

  describe("template interpolation", () => {
    it("WorkflowDetector_should_interpolateTemplate_When_workflowHasInputTemplate", () => {
      // T-UNIT-TS-107
      const d = makeDetector();
      const result = d.detect("@daily-standup team-alpha");
      expect(result!.metadata!.interpolatedPrompt).toBe("Run standup for team-alpha");
    });

    it("WorkflowDetector_should_useArgAsPrompt_When_noInputTemplate", () => {
      // T-UNIT-TS-108
      const workflows: WorkflowEntry[] = [{ slug: "no-template", name: "No Template" }];
      const d = new WorkflowDetector(workflows);
      const result = d.detect("@no-template my-arg");
      expect(result!.metadata!.interpolatedPrompt).toBe("my-arg");
    });

    it("WorkflowDetector_should_substituteAllOccurrences_When_templateHasMultipleInputTokens", () => {
      // T-UNIT-TS-109
      const workflows: WorkflowEntry[] = [
        { slug: "multi", name: "Multi", inputTemplate: "Do {{input}} then {{input}}" },
      ];
      const d = new WorkflowDetector(workflows);
      const result = d.detect("@multi foo");
      expect(result!.metadata!.interpolatedPrompt).toBe("Do foo then foo");
    });
  });

  describe("case insensitivity", () => {
    it("WorkflowDetector_should_matchCaseInsensitive_When_slugTypedInUpperCase", () => {
      // T-UNIT-TS-110
      const d = makeDetector();
      const result = d.detect("@DAILY-STANDUP");
      expect(result!.metadata!.workflowFound).toBe(true);
      expect(result!.metadata!.slug).toBe("daily-standup");
    });
  });

  describe("priority and name", () => {
    it("WorkflowDetector_should_havePriority25", () => {
      // T-UNIT-TS-111
      const d = makeDetector();
      expect(d.priority).toBe(25);
    });

    it("WorkflowDetector_should_haveCorrectName", () => {
      // T-UNIT-TS-112
      expect(makeDetector().name).toBe("WorkflowDetector");
    });
  });

  describe("metadata", () => {
    it("WorkflowDetector_should_includeWorkflowObject_When_slugFound", () => {
      // T-UNIT-TS-113
      const d = makeDetector();
      const result = d.detect("@daily-standup");
      const wf = result!.metadata!.workflow as WorkflowEntry;
      expect(wf.slug).toBe("daily-standup");
      expect(wf.name).toBe("Daily Standup");
    });

    it("WorkflowDetector_should_setSuggestedName_When_workflowFound", () => {
      // T-UNIT-TS-114
      const d = makeDetector();
      const result = d.detect("@daily-standup");
      expect(result!.suggestedName).toBe("Daily Standup");
    });

    it("WorkflowDetector_should_setSuggestedNameToSlug_When_workflowNotFound", () => {
      // T-UNIT-TS-115
      const d = makeDetector();
      const result = d.detect("@unknown");
      expect(result!.suggestedName).toBe("unknown");
    });
  });

  describe("edge cases", () => {
    it("WorkflowDetector_should_returnNull_When_emptyInput", () => {
      // T-UNIT-TS-116
      const d = makeDetector();
      expect(d.detect("")).toBeNull();
    });

    it("WorkflowDetector_should_returnNull_When_onlyAtSign", () => {
      // T-UNIT-TS-117 — '@' alone doesn't match
      const d = makeDetector();
      // Single '@' doesn't match the regex (requires at least one slug char after @)
      expect(d.detect("@")).toBeNull();
    });

    it("WorkflowDetector_should_trimInput_When_inputHasLeadingSpaces", () => {
      // T-UNIT-TS-118
      const d = makeDetector();
      const result = d.detect("  @daily-standup  ");
      expect(result).not.toBeNull();
      expect(result!.metadata!.workflowFound).toBe(true);
    });
  });
});
