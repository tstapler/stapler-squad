import {
  CONNECT_GATE_WIDTH,
  queueForConnectSlot,
  resetConnectGateForTests,
  tryAcquireConnectSlot,
  type ReleaseConnectSlot,
} from "./connectGate";

function fillGate(): ReleaseConnectSlot[] {
  return Array.from({ length: CONNECT_GATE_WIDTH }, () => {
    const release = tryAcquireConnectSlot();
    if (!release) throw new Error("gate unexpectedly full");
    return release;
  });
}

async function settled<T>(p: Promise<T>): Promise<T | "pending"> {
  return Promise.race([p, Promise.resolve("pending" as const)]);
}

describe("connectGate", () => {
  beforeEach(() => resetConnectGateForTests());

  it("tryAcquire_should_returnNull_When_gateIsFull", () => {
    fillGate();
    expect(tryAcquireConnectSlot()).toBeNull();
  });

  it("queue_should_grantInFifoOrder_When_slotsAreReleased", async () => {
    const held = fillGate();
    const first = queueForConnectSlot(false);
    const second = queueForConnectSlot(false);

    held[0]();
    expect(await settled(first.promise)).toEqual(expect.any(Function));
    expect(await settled(second.promise)).toBe("pending");

    held[1]();
    expect(await settled(second.promise)).toEqual(expect.any(Function));
  });

  it("queue_should_grantPriorityWaiterFirst_When_itQueuedLater", async () => {
    const held = fillGate();
    const background = queueForConnectSlot(false);
    const foreground = queueForConnectSlot(true);

    held[0]();
    expect(await settled(foreground.promise)).toEqual(expect.any(Function));
    expect(await settled(background.promise)).toBe("pending");
  });

  it("tryAcquire_should_notBarge_When_waitersAreQueued", async () => {
    const held = fillGate();
    const waiter = queueForConnectSlot(false);

    held[0]();
    // The freed slot went to the waiter inside release(), so a newcomer cannot take it.
    expect(tryAcquireConnectSlot()).toBeNull();
    expect(await settled(waiter.promise)).toEqual(expect.any(Function));
  });

  it("cancel_should_resolveNullAndKeepSlotsFree_When_waiterIsStillQueued", async () => {
    const held = fillGate();
    const cancelled = queueForConnectSlot(false);
    const next = queueForConnectSlot(false);

    cancelled.cancel();
    expect(await cancelled.promise).toBeNull();

    held[0]();
    expect(await settled(next.promise)).toEqual(expect.any(Function));
  });

  it("cancel_should_beNoOp_When_slotWasAlreadyGranted", async () => {
    const held = fillGate();
    const waiter = queueForConnectSlot(false);
    held[0]();
    const release = await waiter.promise;

    waiter.cancel();

    expect(release).toEqual(expect.any(Function));
    expect(tryAcquireConnectSlot()).toBeNull(); // the granted slot is still held
    release?.();
    expect(tryAcquireConnectSlot()).not.toBeNull();
  });

  it("release_should_freeOnlyOneSlot_When_calledTwice", () => {
    const held = fillGate();
    held[0]();
    held[0]();

    expect(tryAcquireConnectSlot()).not.toBeNull();
    expect(tryAcquireConnectSlot()).toBeNull(); // a double release must not mint an extra slot
  });
});
