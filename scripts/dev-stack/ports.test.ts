import * as net from "net";
import { EventEmitter } from "events";
import { allocatePort } from "./ports";

// net's `createServer` export is a non-configurable getter on newer Node
// versions, so jest.spyOn(net, "createServer") can't redefine it directly
// ("Cannot redefine property: createServer"). Swapping the whole module for
// a plain object via jest.mock() sidesteps that — the replacement object's
// properties are configurable, so individual tests below can still install
// mockImplementation(s) on net.createServer while the integration tests
// keep exercising the real implementation by default.
jest.mock("net", () => {
  const actual = jest.requireActual<typeof import("net")>("net");
  return {
    ...actual,
    createServer: jest.fn(actual.createServer),
  };
});

const realCreateServer = jest.requireActual<typeof import("net")>("net").createServer;

/**
 * Attempts to bind a real listener on `port`, retrying briefly on
 * EADDRINUSE. Used to prove release() actually freed the OS-level socket
 * rather than just the in-process record — server.close() schedules the
 * underlying handle close asynchronously, so a same-tick rebind can
 * legitimately race the OS for a few milliseconds.
 */
function bindRealServer(port: number, attemptsLeft = 20): Promise<net.Server> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    server.once("error", (err: NodeJS.ErrnoException) => {
      server.close();
      if (err.code === "EADDRINUSE" && attemptsLeft > 1) {
        setTimeout(() => {
          bindRealServer(port, attemptsLeft - 1).then(resolve, reject);
        }, 25);
        return;
      }
      reject(err);
    });

    server.listen(port, () => resolve(server));
  });
}

/**
 * A fake net.Server for the mocked-createServer unit tests: emits
 * "error" (EADDRINUSE) or "listening" asynchronously, matching the real
 * net.Server's async event timing closely enough for ports.ts's listener
 * ordering (attach handlers, then call listen()) to observe it correctly.
 */
function createFakeServer(opts: { fail?: boolean; port?: number }): net.Server {
  const emitter = new EventEmitter() as unknown as net.Server;

  (emitter as unknown as { listen: (port: number) => net.Server }).listen = (_port: number) => {
    process.nextTick(() => {
      if (opts.fail) {
        const err = new Error("address already in use") as NodeJS.ErrnoException;
        err.code = "EADDRINUSE";
        emitter.emit("error", err);
      } else {
        emitter.emit("listening");
      }
    });
    return emitter;
  };

  (emitter as unknown as { close: () => net.Server }).close = () => emitter;
  (emitter as unknown as { address: () => net.AddressInfo }).address = () => ({
    port: opts.port ?? 0,
    address: "127.0.0.1",
    family: "IPv4",
  });

  return emitter;
}

describe("allocatePort()", () => {
  it("allocatePort() should return distinct ports when called twice back-to-back in the same process", async () => {
    const first = await allocatePort();
    const second = await allocatePort();

    try {
      expect(first.port).not.toBe(second.port);
    } finally {
      first.release();
      second.release();
    }
  });

  it("release() should free the underlying socket so a real server can bind the same port afterward", async () => {
    const allocated = await allocatePort();
    const { port } = allocated;

    allocated.release();

    const rebound = await bindRealServer(port);
    await new Promise<void>((resolve) => rebound.close(() => resolve()));
  });

  it("allocatePort() should retry with a fresh listener when the bind throws EADDRINUSE, and succeed within 3 attempts", async () => {
    const createServerMock = net.createServer as jest.Mock;
    createServerMock
      .mockClear()
      .mockImplementationOnce(() => createFakeServer({ fail: true }))
      .mockImplementationOnce(() => createFakeServer({ fail: true }))
      .mockImplementationOnce(() => createFakeServer({ port: 54321 }));

    try {
      const allocated = await allocatePort();

      expect(allocated.port).toBe(54321);
      expect(createServerMock).toHaveBeenCalledTimes(3);
    } finally {
      createServerMock.mockReset().mockImplementation(realCreateServer);
    }
  });

  it("allocatePort() should throw a clear error when every retry attempt fails", async () => {
    const createServerMock = net.createServer as jest.Mock;
    createServerMock.mockClear().mockImplementation(() => createFakeServer({ fail: true }));

    try {
      await expect(allocatePort()).rejects.toThrow(/failed to allocate a free port after 3 attempts/i);
      expect(createServerMock).toHaveBeenCalledTimes(3);
    } finally {
      createServerMock.mockReset().mockImplementation(realCreateServer);
    }
  });
});
