// jest-environment-jsdom has no fetch API, and React Router's data router builds a `Request` per navigation.
// Borrow Node's own implementations from the outer realm; AbortController must come from there too, because
// Node's Request rejects a jsdom AbortSignal. Import this first in any suite that mounts a data router.
import { Script } from "node:vm";

const outer = (name: string): unknown => new Script(name).runInThisContext();

if (typeof globalThis.Request === "undefined") {
  Object.assign(globalThis, {
    Request: outer("Request"),
    Response: outer("Response"),
    Headers: outer("Headers"),
    AbortController: outer("AbortController"),
    AbortSignal: outer("AbortSignal"),
  });
}
