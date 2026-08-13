import * as net from "net";

/**
 * Ports handed out by allocatePort() during this process's lifetime.
 *
 * Bind-and-hold (see allocatePort()) already makes a same-run collision
 * essentially impossible, since the OS won't hand out a port that's still
 * bound by an open listener. This set is a cheap guard against a future
 * refactor (e.g. an eager-release variant) reintroducing the probe-and-close
 * TOCTOU gap it currently prevents. Ports are intentionally never removed
 * on release() — the goal is "never handed out twice this run", not "never
 * bound twice this run".
 */
const AllocatedPortSet = new Set<number>();

const MAX_ATTEMPTS = 3;

export interface AllocatedPort {
  port: number;
  release: () => void;
}

function isEaddrinuse(err: unknown): boolean {
  return Boolean(err) && typeof err === "object" && (err as NodeJS.ErrnoException).code === "EADDRINUSE";
}

/** Binds a single fresh listener to an OS-assigned port. Does not close it. */
function bindOnce(): Promise<{ port: number; server: net.Server }> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();

    const onError = (err: unknown) => {
      server.removeListener("listening", onListening);
      reject(err);
    };

    const onListening = () => {
      server.removeListener("error", onError);
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("allocatePort(): listen(0) did not return an AddressInfo"));
        return;
      }
      resolve({ port: address.port, server });
    };

    server.once("error", onError);
    server.once("listening", onListening);
    server.listen(0);
  });
}

/**
 * Allocates a free TCP port by binding a real listener and holding it open
 * (bind-and-hold) rather than probing a port and immediately closing it
 * (probe-and-close). The caller is responsible for calling release() —
 * ideally immediately before spawning the real child process that will bind
 * this exact port — which closes the listener and frees the OS-level socket.
 *
 * Retries (fresh net.createServer() each time) up to MAX_ATTEMPTS when the
 * bind fails with EADDRINUSE, or when the newly bound port collides with one
 * already handed out this run (see AllocatedPortSet above).
 */
export async function allocatePort(): Promise<AllocatedPort> {
  let lastError: unknown;

  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      const { port, server } = await bindOnce();

      if (AllocatedPortSet.has(port)) {
        lastError = new Error(
          `allocatePort(): port ${port} was already reserved this run (attempt ${attempt}/${MAX_ATTEMPTS})`,
        );
        server.close();
        continue;
      }

      AllocatedPortSet.add(port);

      let released = false;
      const release = () => {
        if (released) return;
        released = true;
        server.close();
      };

      return { port, release };
    } catch (err) {
      lastError = err;
      if (!isEaddrinuse(err)) {
        throw err;
      }
      // EADDRINUSE — retry with a fresh listener.
    }
  }

  const reason = lastError instanceof Error ? lastError.message : String(lastError);
  throw new Error(`allocatePort(): failed to allocate a free port after ${MAX_ATTEMPTS} attempts: ${reason}`);
}
