import {
  pendingQuestionFor,
  replyAvailability,
  truncatePrompt,
  unavailableCopy,
  PROMPT_TRUNCATE_CHARS,
} from "../replyQuestion";

const question = (md: Record<string, string> | undefined, extra = {}) => ({
  id: "n1",
  sessionId: "s1",
  sessionName: "s1",
  message: "Which color should the spike use?",
  timestamp: 1,
  notificationType: "question" as const,
  isRead: false,
  metadata: md,
  ...extra,
});

describe("replyAvailability", () => {
  it("is_replyable_for_a_single_select_question_with_id_shape_and_options", () => {
    const a = replyAvailability(
      question({ question_id: "q1", question_shape: "single", question_options: '["Red","Green"]' }),
    );
    expect(a).toEqual({ kind: "replyable", questionId: "q1", options: ["Red", "Green"], prompt: "Which color should the spike use?" });
  });

  it.each([[2], [5]])("is_unavailable_parallel_question_when_occurrenceCount_is_%i", (occurrenceCount) => {
    const a = replyAvailability(
      question(
        { question_id: "q1", question_shape: "single", question_options: '["Red","Green"]' },
        { occurrenceCount },
      ),
    );
    expect(a).toEqual({ kind: "unavailable", cause: "parallel_question" });
    expect(unavailableCopy("parallel_question")).toBe("Answer in the terminal");
  });

  it("is_unavailable_when_the_server_reports_more_than_one_pending_question", () => {
    const a = replyAvailability(
      question({ question_id: "q1", question_shape: "single", question_options: '["a"]', question_pending_count: "2" }),
    );
    expect(a).toEqual({ kind: "unavailable", cause: "parallel_question" });
  });

  it.each([["0"], [""], ["abc"], ["-1"], ["1.5"]])(
    "falls_back_to_occurrenceCount_when_question_pending_count_is_%p",
    (pending) => {
      const md = { question_id: "q1", question_shape: "single", question_options: '["a"]', question_pending_count: pending };
      expect(replyAvailability(question(md, { occurrenceCount: 1 }))?.kind).toBe("replyable");
      expect(replyAvailability(question(md, { occurrenceCount: 3 }))).toEqual({
        kind: "unavailable",
        cause: "parallel_question",
      });
    },
  );

  it("stays_replyable_when_one_question_is_pending_however_many_were_answered_before", () => {
    const a = replyAvailability(
      question(
        { question_id: "q1", question_shape: "single", question_options: '["a"]', question_pending_count: "1" },
        { occurrenceCount: 7 },
      ),
    );
    expect(a?.kind).toBe("replyable");
  });

  it.each([[1], [undefined]])("stays_replyable_when_occurrenceCount_is_%s", (occurrenceCount) => {
    const a = replyAvailability(
      question(
        { question_id: "q1", question_shape: "single", question_options: '["Red","Green"]' },
        { occurrenceCount },
      ),
    );
    expect(a?.kind).toBe("replyable");
  });

  it("is_null_for_a_notification_that_is_not_a_question", () => {
    expect(replyAvailability({ notificationType: "error", metadata: {}, message: "x" })).toBeNull();
  });

  it.each([
    ["multi shape", { question_id: "q", question_shape: "multi", question_options: '["a"]' }],
    ["unknown shape", { question_id: "q", question_shape: "unknown", question_options: '["a"]' }],
    ["no question id", { question_shape: "single", question_options: '["a"]' }],
    ["bad options json", { question_id: "q", question_shape: "single", question_options: "not json" }],
    ["empty options", { question_id: "q", question_shape: "single", question_options: "[]" }],
    ["eight options", { question_id: "q", question_shape: "single", question_options: JSON.stringify(Array(8).fill("x")) }],
    ["blank label", { question_id: "q", question_shape: "single", question_options: '["a"," "]' }],
    ["no metadata", undefined],
  ])("is_unavailable_with_the_terminal_copy_for_%s", (_name, md) => {
    const a = replyAvailability(question(md as Record<string, string> | undefined));
    expect(a).toEqual({ kind: "unavailable", cause: "shape" });
    expect(unavailableCopy("shape")).toBe("Answer in the terminal");
  });

  it("carries_the_hook_proof_causes_with_their_own_copy", () => {
    for (const cause of ["no_proof", "bad_proof"] as const) {
      const a = replyAvailability(question({ reply_unavailable: cause, question_shape: "single" }));
      expect(a).toEqual({ kind: "unavailable", cause });
      expect(unavailableCopy(cause)).toBe("Reply unavailable for this session. Answer in the terminal.");
    }
  });
});

describe("truncatePrompt", () => {
  it("keeps_a_short_prompt_and_cuts_a_long_one_at_280_characters", () => {
    expect(truncatePrompt("short")).toEqual({ shown: "short", truncated: false });
    const long = "x".repeat(PROMPT_TRUNCATE_CHARS + 20);
    const t = truncatePrompt(long);
    expect(t.truncated).toBe(true);
    expect(Array.from(t.shown)).toHaveLength(PROMPT_TRUNCATE_CHARS);
  });
});

describe("pendingQuestionFor", () => {
  it("returns_the_newest_unread_question_of_the_session_only", () => {
    const history = [
      question({}, { id: "old", timestamp: 1 }),
      question({}, { id: "new", timestamp: 5 }),
      question({}, { id: "read", timestamp: 9, isRead: true }),
      question({}, { id: "other", timestamp: 7, sessionId: "s2" }),
      question({}, { id: "err", timestamp: 8, notificationType: "error" }),
    ];
    expect(pendingQuestionFor(history, "s1")?.id).toBe("new");
    expect(pendingQuestionFor(history, "s1", "old")?.id).toBe("old");
    expect(pendingQuestionFor(history, "nobody")).toBeUndefined();
  });
});
