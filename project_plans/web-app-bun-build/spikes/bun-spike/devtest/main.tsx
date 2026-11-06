import React from "react"; import { createRoot } from "react-dom/client";
import { ErrorState } from "../src/components/ui/ErrorState";
createRoot(document.getElementById("root")!).render(<ErrorState title="Bun dev" message="hello" />);
