import { render, screen } from "@testing-library/react";
import { MemoryNavProvider, usePathname, useSearchParams } from "@/lib/nav/impl.memory";
import ConfigEditorPage from "../page";
import DefaultsPage from "../../settings/defaults/page";

// The global @/lib/nav mock stubs the hooks; the memory impl is the real in-memory router.
function Location() {
  return <output data-testid="location">{`${usePathname()}?${useSearchParams().toString()}`}</output>;
}

function renderAt(entry: string, page: React.ReactElement) {
  render(
    <MemoryNavProvider initialEntries={[entry]}>
      {page}
      <Location />
    </MemoryNavProvider>,
  );
}

describe("redirect-only pages", () => {
  it("/config?x=1 redirects to /settings?tab=config-files and drops x", () => {
    renderAt("/config?x=1", <ConfigEditorPage />);
    expect(screen.getByTestId("location").textContent).toBe("/settings?tab=config-files");
  });

  it("/settings/defaults redirects to /settings", () => {
    renderAt("/settings/defaults", <DefaultsPage />);
    expect(screen.getByTestId("location").textContent).toBe("/settings?");
  });
});
