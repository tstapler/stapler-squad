import { createApi } from "@reduxjs/toolkit/query/react";
import type { BaseQueryFn } from "@reduxjs/toolkit/query";
import { ConnectError } from "@connectrpc/connect";

// Generic base query for ConnectRPC unary calls.
// arg shape: { call: () => Promise<unknown> }
const connectBaseQuery: BaseQueryFn<
  { call: () => Promise<unknown> },
  unknown,
  { status: number; error: string }
> = async ({ call }) => {
  try {
    const result = await call();
    return { data: result };
  } catch (err) {
    if (err instanceof ConnectError) {
      // Store rawMessage, not message — .message is ConnectRPC's
      // `[code] message`-prefixed string, and callers reading this error
      // slice (e.g. BacklogItemDetail.tsx's lastError?.message reads)
      // display it verbatim to users.
      return { error: { status: err.code, error: err.rawMessage || err.message } };
    }
    const msg = err instanceof Error ? err.message : "Unknown error";
    return { error: { status: -1, error: msg } };
  }
};

export const connectApi = createApi({
  reducerPath: "connectApi",
  baseQuery: connectBaseQuery,
  tagTypes: ["Approvals"],
  endpoints: () => ({}),
});
