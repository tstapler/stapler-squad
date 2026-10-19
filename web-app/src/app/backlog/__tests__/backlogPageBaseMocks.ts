/**
 * Side-effect import: registers the baseline BacklogPage module mocks. Import it BEFORE
 * the page under test; jest.doMock is not hoisted, so import order makes it take effect.
 */
/* eslint-disable @typescript-eslint/no-require-imports */
const fixtures = () => require("./backlogPageTestFixtures");

jest.doMock("next/navigation", () => fixtures().nextNavigationMock());
jest.doMock("@/lib/analytics", () => fixtures().analyticsMock());
jest.doMock("@/lib/analytics/usePageView", () => fixtures().usePageViewMock());
jest.doMock("@/components/backlog/BacklogItemDetail", () => fixtures().backlogItemDetailMock());
jest.doMock("@/components/backlog/BacklogItemForm", () => fixtures().backlogItemFormMock());
jest.doMock("@/components/backlog/BacklogEmptyState", () => fixtures().backlogEmptyStateMock());
jest.doMock("@/components/backlog/VaguenessPromptModal", () => fixtures().vaguenessPromptModalMock());
jest.doMock("@/components/backlog/BacklogTourModal", () => fixtures().backlogTourModalMock());
jest.doMock("@/components/backlog/GitHubIssuePicker", () => fixtures().gitHubIssuePickerMock());
jest.doMock("@connectrpc/connect", () => fixtures().connectMock());
jest.doMock("@connectrpc/connect-web", () => fixtures().connectWebMock());
jest.doMock("@/lib/config", () => fixtures().configMock());
jest.doMock("@/lib/store", () => fixtures().storeMock());
jest.doMock("@/lib/hooks/useBacklogService", () => fixtures().useBacklogServiceMockFactory());
jest.doMock("@/lib/hooks/useWatchBacklogItems", () => fixtures().useWatchBacklogItemsMock());
