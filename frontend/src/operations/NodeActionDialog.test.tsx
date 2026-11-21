// @vitest-environment happy-dom
/*
Purpose: Verify Node review ownership and consent with the real mounted dialog.
Used by: The workspace frontend regression suite.
Responsibilities: Check typed acknowledgement, settings invalidation and blocked reviews.
Important: The bridge returns only synthetic fixture data.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { test, expect, vi } from "vitest";
import { NodeActionDialog } from "./NodeActionDialog";
import type { ResourceDetail } from "../workloads/model";
const api = vi.hoisted(() => ({ prepareNode: vi.fn(), revokeNode: vi.fn() }));
vi.mock("./api", () => api);
vi.mock("../metrics/HistoricalUsage", () => ({ HistoricalUsage: () => null }));
vi.mock("@wailsio/runtime", () => ({}));
test("changing wait revokes consent and a blocked Pod cannot be confirmed", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const source = {
    resource: { kind: "Node", apiVersion: "v1", name: "test-node", uid: "captured", namespace: "", connection: { id: "a", generation: 1 } },
    resourceVersion: "7",
  } as ResourceDetail;
  const execute = vi.fn(),
    preview = {
      rows: [],
      counts: {},
      typed: true,
      cordoned: false,
      expiresAt: new Date(Date.now() + 60000).toISOString(),
      token: "approved",
    };
  api.prepareNode.mockReturnValueOnce({ promise: Promise.resolve(preview), cancel: vi.fn() }).mockReturnValueOnce({
    promise: Promise.resolve({
      ...preview,
      token: "",
      counts: { blocked: 1 },
      rows: [{ state: "blocked", namespace: "other", name: "pod", uid: "p", reason: "Temporary Pod storage (emptyDir)" }],
    }),
    cancel: vi.fn(),
  });
  const button = (name: string) => Array.from(document.querySelectorAll("button")).find((b) => b.textContent === name);
  try {
    await act(async () =>
      root.render(<NodeActionDialog source={source} cluster="A" action="drain" onDismiss={() => {}} onExecute={execute} />),
    );
    await act(async () => button("Prepare review")!.click());
    expect(document.body.textContent).toContain("across all namespaces");
    expect(document.body.textContent).toContain("stays cordoned");
    expect(button("Confirm Drain")!.disabled).toBe(true);
    const input = document.querySelector("input")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "test-node");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(button("Confirm Drain")!.disabled).toBe(false);
    const select = document.querySelector("select")!;
    await act(async () => {
      select.value = "2";
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    expect(api.revokeNode).toHaveBeenCalledWith("approved");
    expect(button("Confirm Drain")).toBeUndefined();
    await act(async () => button("Prepare review")!.click());
    expect(api.prepareNode).toHaveBeenLastCalledWith(
      expect.objectContaining({ waitPreset: 2, sourceVersion: "7", resource: source.resource }),
    );
    expect(document.body.textContent).toContain("other/pod");
    expect(document.body.textContent).toContain("emptyDir");
    expect(button("Confirm Drain")).toBeUndefined();
    expect(execute).not.toHaveBeenCalled();
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});

test("same-name connection switch revokes a late review and stale detail has no Node menu", async () => {
  const { ResourceDetail: Detail } = await import("../workspace/ResourceDetail");
  const { emptyKind } = await import("../workloads/state");
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const render = (id: string, stale = false) => {
    const resource = { kind: "Node", apiVersion: "v1", name: "same", uid: id, namespace: "", connection: { id, generation: 1 } },
      data = { resource, resourceVersion: "7", objectJSON: '{"spec":{}}' };
    const controller = {
      selection: { resource },
      activeSection: "Overview",
      sections: { Overview: { data } },
      inventory: emptyKind(),
    } as unknown as import("../workloads/useWorkloads").WorkloadController;
    root.render(<Detail controller={controller} context={id} stale={stale} canReviewOperation={!stale} onOperation={vi.fn()} />);
  };
  let resolve!: (v: { token: string }) => void;
  const cancel = vi.fn();
  api.prepareNode.mockReturnValueOnce({
    promise: new Promise((r) => {
      resolve = r;
    }),
    cancel,
  });
  try {
    await act(async () => render("a"));
    const menu = host.querySelector<HTMLSelectElement>('select[aria-label="Node actions"]')!;
    expect([...menu.options].map((o) => o.value)).toEqual(["", "cordon", "drain"]);
    await act(async () => {
      menu.value = "drain";
      menu.dispatchEvent(new Event("change", { bubbles: true }));
    });
    await act(async () => [...host.querySelectorAll("button")].find((b) => b.textContent === "Prepare review")!.click());
    await act(async () => render("b"));
    expect(cancel).toHaveBeenCalledOnce();
    await act(async () => resolve({ token: "late-a" }));
    expect(api.revokeNode).toHaveBeenCalledWith("late-a");
    expect(host.querySelector("dialog")).toBeNull();
    await act(async () => render("b", true));
    expect(host.querySelector('[aria-label="Node actions"]')).toBeNull();
  } finally {
    await act(async () => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  }
});
