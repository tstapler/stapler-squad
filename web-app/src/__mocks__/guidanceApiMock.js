// Global mock for lib/api/guidanceApi in Jest, mapped via jest.config.js's
// moduleNameMapper. GuidanceRequestPanel (AC3) uses these RTK Query hooks,
// which need a real Redux <Provider> that most component test suites never
// set up — a per-suite jest.mock("@/lib/api/guidanceApi", ...) would have to
// be added to every existing and future suite that renders BacklogItemDetail,
// TriageReviewPanel, or SessionDetailView, so this is mapped once instead. A
// suite that wants realistic guidance-request data can still override this
// with its own explicit jest.mock("@/lib/api/guidanceApi", ...) call.
module.exports = {
  useListGuidanceRequestsQuery: () => ({ data: undefined, isLoading: false, error: null }),
  useAnswerGuidanceRequestMutation: () => [() => Promise.resolve({ data: undefined }), {}],
};
