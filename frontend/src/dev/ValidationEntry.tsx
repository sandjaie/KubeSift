/*
Purpose: Preserve developer diagnostics outside the packaged product route.
Used by: Vite development's validation.html entry only.
Responsibilities: Mount the retained Phase 1A harness and make synthetic fixtures inspectable.
Important: Production index does not import this module, its controller or fixtures.
*/
import { createRoot } from "react-dom/client";
import ValidationApp from "../ValidationApp";
import { createPreview } from "../preview/fixtures";
if (import.meta.env.DEV) {
  createRoot(document.getElementById("root")!).render(
    <>
      <div className="validation-workspace">
        <ValidationApp />
      </div>
      <details>
        <summary>Synthetic UI fixture inventory · developer only</summary>
        <pre>{JSON.stringify(createPreview(), null, 2)}</pre>
      </details>
    </>,
  );
}
