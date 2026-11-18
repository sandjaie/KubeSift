// @vitest-environment happy-dom
/*
Purpose: Preserve connection action clicks when WebKit blurs a disclosure without a focus target.
Used by: Frontend connection-control regression tests.
Responsibilities: Keep pointer actions available and close after keyboard focus moves outside.
Important: Native WebKit verification complements the synthetic focus events.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import { ConnectionControls } from "./ConnectionControls";
import type { ConnectionTab } from "./model";

test("null-target blur preserves connection actions; external focus closes them", async () => {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const retry = vi.fn();
  const tab = { entry: { context: "kind-dev-local-a", sourceDisplay: "fixture" }, view: { state: "connected" } } as ConnectionTab;
  try {
    await act(async () => root.render(<ConnectionControls tab={tab} onRetry={retry} onCancel={vi.fn()} onReview={vi.fn()} />));
    const details = host.querySelector("details")!;
    const summary = host.querySelector("summary")!;
    details.open = true;
    await act(async () => summary.dispatchEvent(new FocusEvent("focusout", { bubbles: true, relatedTarget: null })));
    expect(details.open).toBe(true);
    await act(async () => details.querySelector("button")!.click());
    expect(retry).toHaveBeenCalledOnce();
    expect(details.open).toBe(false);
    details.open = true;
    await act(async () => summary.dispatchEvent(new FocusEvent("focusout", { bubbles: true, relatedTarget: document.body })));
    expect(details.open).toBe(false);
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
