import { DetectedStatus, SubStatus, WorkingState } from "@/gen/session/v1/types_pb";
import { deriveWorkingState } from "../deriveWorkingState";

// ---------------------------------------------------------------------------
// SubStatus-based mappings (primary signal)
// ---------------------------------------------------------------------------

describe("deriveWorkingState — SubStatus.PROCESSING group", () => {
  const processingStates: SubStatus[] = [
    SubStatus.PROCESSING,
    SubStatus.NEEDS_APPROVAL,
    SubStatus.INPUT_REQUIRED,
    SubStatus.ERROR,
    SubStatus.TESTS_FAILING,
    SubStatus.RATE_LIMITED,
    SubStatus.WAITING_FOR_AGENT,
  ];

  for (const subStatus of processingStates) {
    it(`deriveWorkingState_should_returnPROCESSING_When_subStatus_is_${SubStatus[subStatus]}`, () => {
      expect(deriveWorkingState({ subStatus })).toBe(WorkingState.PROCESSING);
    });
  }
});

describe("deriveWorkingState — SubStatus.IDLE group", () => {
  const idleStates: SubStatus[] = [
    SubStatus.IDLE,
    SubStatus.READY,
    SubStatus.SUCCESS,
  ];

  for (const subStatus of idleStates) {
    it(`deriveWorkingState_should_returnIDLE_When_subStatus_is_${SubStatus[subStatus]}`, () => {
      expect(deriveWorkingState({ subStatus })).toBe(WorkingState.IDLE);
    });
  }
});

// ---------------------------------------------------------------------------
// DetectedStatus fallback (when SubStatus.UNSPECIFIED)
// ---------------------------------------------------------------------------

describe("deriveWorkingState — detectedStatus fallback (SubStatus.UNSPECIFIED)", () => {
  it("deriveWorkingState_should_returnACTIVE_When_detectedStatus_is_EXECUTING", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.EXECUTING,
      })
    ).toBe(WorkingState.ACTIVE);
  });

  it("deriveWorkingState_should_returnACTIVE_When_detectedStatus_is_WAITING_FOR_AGENT", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.WAITING_FOR_AGENT,
      })
    ).toBe(WorkingState.ACTIVE);
  });

  it("deriveWorkingState_should_returnPROCESSING_When_detectedStatus_is_PROCESSING", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.PROCESSING,
      })
    ).toBe(WorkingState.PROCESSING);
  });

  it("deriveWorkingState_should_returnIDLE_When_detectedStatus_is_IDLE", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.IDLE,
      })
    ).toBe(WorkingState.IDLE);
  });

  it("deriveWorkingState_should_returnIDLE_When_detectedStatus_is_READY", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.READY,
      })
    ).toBe(WorkingState.IDLE);
  });

  it("deriveWorkingState_should_returnUNSPECIFIED_When_detectedStatus_is_UNKNOWN", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.UNKNOWN,
      })
    ).toBe(WorkingState.UNSPECIFIED);
  });

  it("deriveWorkingState_should_returnUNSPECIFIED_When_detectedStatus_is_absent", () => {
    expect(
      deriveWorkingState({ subStatus: SubStatus.UNSPECIFIED })
    ).toBe(WorkingState.UNSPECIFIED);
  });

  it("deriveWorkingState_should_returnUNSPECIFIED_When_detectedStatus_is_UNSPECIFIED", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.UNSPECIFIED,
        detectedStatus: DetectedStatus.UNSPECIFIED,
      })
    ).toBe(WorkingState.UNSPECIFIED);
  });
});

// ---------------------------------------------------------------------------
// SubStatus takes precedence over detectedStatus
// ---------------------------------------------------------------------------

describe("deriveWorkingState — SubStatus precedence", () => {
  it("deriveWorkingState_should_ignoreDerectedStatus_When_subStatus_is_set", () => {
    // Even if detectedStatus says EXECUTING, subStatus.IDLE wins
    expect(
      deriveWorkingState({
        subStatus: SubStatus.IDLE,
        detectedStatus: DetectedStatus.EXECUTING,
      })
    ).toBe(WorkingState.IDLE);
  });

  it("deriveWorkingState_should_returnPROCESSING_When_subStatus_PROCESSING_and_detectedStatus_IDLE", () => {
    expect(
      deriveWorkingState({
        subStatus: SubStatus.PROCESSING,
        detectedStatus: DetectedStatus.IDLE,
      })
    ).toBe(WorkingState.PROCESSING);
  });
});
