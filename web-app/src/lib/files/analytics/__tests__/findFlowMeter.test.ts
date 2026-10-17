import { FindFlowMeter } from "../findFlowMeter";

describe("FindFlowMeter", () => {
  it("findFlowMeter_should_Report3ThenReset_When_OpenTypeTapThenCancelled", () => {
    const meter = new FindFlowMeter();
    meter.open();
    meter.interact();
    expect(meter.finish()).toBe(3);
    expect(meter.finish()).toBeNull();

    meter.open();
    meter.interact();
    meter.interact();
    expect(meter.finish()).toBe(4);

    meter.open();
    meter.interact();
    meter.cancel();
    expect(meter.finish()).toBeNull();
  });

  it("findFlowMeter_should_IgnoreInteractions_When_NoFlowIsRunning", () => {
    const meter = new FindFlowMeter();
    meter.interact();
    expect(meter.finish()).toBeNull();
  });
});
