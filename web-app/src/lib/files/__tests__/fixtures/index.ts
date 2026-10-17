import "./mockTransport"; // must load before the unit under test

export { createFilesTestHarness } from "./filesHarness";
export type { FilesHarness, FilesHarnessOverrides, SessionHandlers } from "./filesHarness";
export { createFakeAnalytics } from "./fakeAnalytics";
export type { FakeAnalytics } from "./fakeAnalytics";
export { HARNESS_MOCKED_MODULES, findDuplicateMocks } from "./mockGuard";
export { installJsdomLayoutShims, installVirtualViewport } from "./jsdomLayout";
export { CANARIES, LABEL_VALUE_SHAPE, MAX_LABELS, canaryLeaks, labelProblems, fixtureLabels } from "./privacyHarness";
export { fakeClock } from "./fakeClock";
export { createFakeTransport, fakeCapabilities, DEFAULT_OPS } from "./fakeTransport";
export type { FakeTransport, FakeTransportOptions } from "./fakeTransport";
