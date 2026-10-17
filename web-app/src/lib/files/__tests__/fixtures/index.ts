import "./mockTransport"; // must load before the unit under test

export { createFilesTestHarness } from "./filesHarness";
export type { FilesHarness, FilesHarnessOverrides, SessionHandlers } from "./filesHarness";
export { createFakeAnalytics } from "./fakeAnalytics";
export type { FakeAnalytics } from "./fakeAnalytics";
export { HARNESS_MOCKED_MODULES, findDuplicateMocks } from "./mockGuard";
export { installJsdomLayoutShims } from "./jsdomLayout";
