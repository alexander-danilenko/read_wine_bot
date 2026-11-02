import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import "./index.css";
import { applyTheme } from "./theme";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

applyTheme(); // before the first paint, so the page never flashes in the wrong colors

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
