import "./app/fonts.css";
import { box } from "./styles/theme.css";
import { greeting } from "./generated/a";

// Each define the build must inline appears as a literal read here.
export const env = [
  process.env.NODE_ENV,
  process.env.NEXT_PUBLIC_API_URL,
  process.env.NEXT_PUBLIC_RECONNECT_V2,
  process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING,
  process.env.NEXT_PUBLIC_OTEL_ENABLED,
  process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT,
];
document.getElementById("root")!.className = box;
document.getElementById("root")!.textContent = `${greeting} ${JSON.stringify(env)}`;
void import("./lazy");
