// Compiled by tsc only (`bunx tsc --noEmit`): each @ts-expect-error line must fail to compile,
// so the build breaks if the registry ever loosens.
import { createFilesEmitter } from "../filesAnalytics";

const a = createFilesEmitter(() => undefined);
const anyString: string = "strip";
const offset: number = 4096;

a.emit("files.tab.switch", { via: "strip" });
a.emit("files.tab.evict_undo");
a.time("files.perf.save_ms")({ outcome: "saved", size: "lt10k" });

// @ts-expect-error unknown event name
a.emit("files.tab.teleport", { via: "strip" });
// @ts-expect-error label value outside its union
a.emit("files.tab.switch", { via: "teleport" });
// @ts-expect-error string-typed label
a.emit("files.tab.switch", { via: anyString });
// @ts-expect-error numeric offset label
a.emit("files.read.window", { transport: "full", payload: offset, outcome: "ok", cache: "hit" });
// @ts-expect-error label key outside the registry
a.emit("files.tab.switch", { via: "strip", path: "secret/dir/Canary.go" });
// @ts-expect-error missing required label
a.emit("files.tab.switch", {});
// @ts-expect-error server events are recorded by Go, not the browser
a.emit("files.server.patch", { outcome: "ok", tier: "t0", transport: "full", payload: "lt1k", ops: "1" });
// @ts-expect-error aggregated event has its own path
a.emit("files.mobile.extra_key_used", { key_class: "esc", count: "1" });
// @ts-expect-error only performance events are timed
a.time("files.tab.switch");
