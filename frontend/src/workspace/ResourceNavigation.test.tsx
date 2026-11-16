// @vitest-environment happy-dom
/*
Purpose: Verify resource sidebar disclosures and selection.
Used by: Frontend workspace regression tests.
Responsibilities: Keep each section independent and preserve the selected destination while hidden.
Important: Uses mounted sidebar markup and no Kubernetes connection.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import { ResourceNavigation, destinations } from "./ResourceNavigation";

test("sidebar sections hide and restore their own destinations without changing selection", async () => {
  const host = document.createElement("div");
  const root = createRoot(host);
  const toggle = vi.fn();
  const select = vi.fn();
  const render = (collapsedGroups: string[]) =>
    act(async () =>
      root.render(<ResourceNavigation kind="Pod" onSelect={select} collapsedGroups={collapsedGroups} onToggleGroup={toggle} />),
    );
  try {
    await render([]);
    const groups = host.querySelectorAll<HTMLButtonElement>("h3 button");
    expect(groups).toHaveLength(destinations.length);
    expect([...groups].every((group) => group.getAttribute("aria-expanded") === "true")).toBe(true);
    const workloads = groups[0];
    const networking = groups[1];
    const list = host.querySelector<HTMLElement>(`[id="${workloads.getAttribute("aria-controls")}"]`);
    workloads.click();
    expect(toggle).toHaveBeenCalledWith("Workloads");
    await render(["Workloads"]);
    expect(workloads.getAttribute("aria-expanded")).toBe("false");
    expect(workloads.hasAttribute("data-current")).toBe(true);
    expect(list?.hidden).toBe(true);
    expect(networking.getAttribute("aria-expanded")).toBe("true");
    expect(select).not.toHaveBeenCalled();
    await render([]);
    expect(list?.hidden).toBe(false);
    const selected = list?.querySelector<HTMLButtonElement>('[aria-pressed="true"]');
    expect(selected?.textContent).toBe("Pods");
    selected?.click();
    expect(select).toHaveBeenCalledWith("Pod", true);
  } finally {
    await act(async () => root.unmount());
  }
});
