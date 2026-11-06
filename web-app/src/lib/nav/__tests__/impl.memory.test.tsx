import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { MemoryNavProvider, memoryImpl } from "../impl.memory";
import { navContract } from "./nav-contract";

navContract({
  name: "impl.memory",
  impl: memoryImpl,
  mount(ui: ReactElement, url: string) {
    window.history.pushState(null, "", url);
    render(<MemoryNavProvider initialEntries={[url]}>{ui}</MemoryNavProvider>);
  },
  keptScroll: () => false,
  redirectedTo: (to, shown) => shown === to,
  // The in-memory fake has no scroll to keep, and its useReplaceSearch is the one impl whose hooks follow the raw write.
  observes: { linkClick: true, replaceSearchHooks: true, scrollOption: false },
});
