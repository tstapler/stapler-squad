/**
 * Side-effect import: registers the standard SessionDetailView child/module mocks
 * (Browser tab = simple, summary panel = null). Import it BEFORE the component under
 * test; jest.doMock is not hoisted, so import order is what makes it take effect.
 * `useWorkflows` is left to each test because the workflow list differs per file.
 */
/* eslint-disable @typescript-eslint/no-require-imports */
const fixtures = () => require("./sessionDetailViewTestFixtures");

jest.doMock("next/dynamic", () => fixtures().mockNextDynamic());
jest.doMock("../DiffViewer", () => fixtures().mockDiffViewer());
jest.doMock("../VcsPanel", () => fixtures().mockVcsPanel());
jest.doMock("../SessionLogsTab", () => fixtures().mockSessionLogsTab());
jest.doMock("../FilesTab", () => fixtures().mockFilesTab());
jest.doMock("../ArtifactsTab", () => fixtures().mockArtifactsTab());
jest.doMock("../WorkspaceSwitchModal", () => fixtures().mockWorkspaceSwitchModal());
jest.doMock("../TagEditor", () => fixtures().mockTagEditor());
jest.doMock("../ResumeSessionModal", () => fixtures().mockResumeSessionModal());
jest.doMock("../BrowserTab", () => fixtures().mockBrowserTabSimple());
jest.doMock("../SessionSummaryPanel", () => fixtures().mockSessionSummaryPanelNull());
jest.doMock("../HandoffSummarySection", () => fixtures().mockHandoffSummarySection());
jest.doMock("@/components/ui/ActionBar", () => fixtures().mockActionBar());
jest.doMock("@/components/ui/Modal", () => fixtures().mockModal());
jest.doMock("@/lib/config", () => fixtures().mockLibConfig());
jest.doMock("@/lib/constants/programs", () => fixtures().mockConstantsPrograms());
jest.doMock("@/lib/store", () => fixtures().mockStore());
jest.doMock("@/lib/store/sessionsSlice", () => fixtures().mockSessionsSlice());
jest.doMock("@/lib/hooks/useShells", () => fixtures().mockUseShells());

export {};
