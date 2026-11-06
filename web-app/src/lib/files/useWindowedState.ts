"use client";

import { useSyncExternalStore } from "react";
import { EMPTY_WINDOWED_STATE, type WindowedFile, type WindowedState } from "./windowedFile";

const noop = () => () => undefined;
const empty = () => EMPTY_WINDOWED_STATE;

/** Live window state of a windowed file (loads, failures, index, count); the empty state for no file. */
export function useWindowedState(file: WindowedFile | null | undefined): WindowedState {
  return useSyncExternalStore(file?.subscribe ?? noop, file?.getSnapshot ?? empty, empty);
}
