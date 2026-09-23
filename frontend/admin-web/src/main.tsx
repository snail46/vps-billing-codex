import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { I18nProvider } from "@vps-billing/i18n";
import "@vps-billing/ui/styles.css";

import { App } from "./App";

const root = document.getElementById("root");
if (!root) {
  throw new Error("root element is missing");
}

createRoot(root).render(
  <StrictMode>
    <I18nProvider>
      <App />
    </I18nProvider>
  </StrictMode>,
);
