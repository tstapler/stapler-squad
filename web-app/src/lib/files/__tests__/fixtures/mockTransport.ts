// Side-effect module: importing it (before the unit under test) replaces the app's Connect
// transport singleton with the in-memory transport held in `fakeTransportRef`.
import type { Transport } from "@connectrpc/connect";

export const fakeTransportRef: { current: Transport | null } = { current: null };

jest.mock("@/lib/api/transport", () => ({
  getConnectTransport: () => {
    if (!fakeTransportRef.current) throw new Error("no fake transport: call createFilesTestHarness() first");
    return fakeTransportRef.current;
  },
}));
