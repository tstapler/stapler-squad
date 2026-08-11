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
      return { error: { status: err.code, error: err.message } };
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
