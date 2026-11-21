// @vitest-environment happy-dom
/*
Purpose: Exercise cross-tab Project verification through the production connection controller.
Used by: Mounted scope-switch regressions.
Responsibilities: Guard delayed reads, same-name identities and failed namespace transitions.
Important: Two synthetic tabs share the same namespace string.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import { useConnections, type ConnectionsController } from "../connections/useConnections";
import { tabKey } from "../connections/state";
import type { CatalogEntry, ConnectionView } from "../connections/api";
const api = vi.hoisted(() => ({
  ListSources: vi.fn().mockResolvedValue([]),
  ReadProject: vi.fn(),
  SetWorkloadNamespace: vi.fn(),
  CancelAttempt: vi.fn().mockResolvedValue(undefined),
  GetAttempt: vi.fn(),
}));
vi.mock("../connections/api", () => ({ api, message: String }));
test("delayed verification cannot switch a different tab; a same-name selection binds its UID only after success", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const root = createRoot(document.createElement("div"));
  let c!: ConnectionsController;
  function Harness() {
    c = useConnections();
    return null;
  }
  await act(async () => root.render(<Harness />));
  const seed = (id: string) => {
    const entry = { sourceId: id, context: id, namespace: "team" } as CatalogEntry;
    const key = tabKey(entry);
    const view = {
      sourceId: id,
      context: id,
      namespace: "team",
      ref: { id, generation: 1 },
      attemptId: id,
      state: "connected",
      pods: [],
    } as unknown as ConnectionView;
    c.commit({ type: "open", entry, request: 1 });
    c.commit({ type: "attempt", key, id, request: 1 });
    c.commit({ type: "snapshot", key, epoch: 0, view });
    return { key, view };
  };
  try {
    let a!: ReturnType<typeof seed>, b!: ReturnType<typeof seed>;
    await act(async () => {
      a = seed("A");
      b = seed("B");
      c.commit({ type: "focus", key: a.key });
    });
    let resolve!: (v: unknown) => void;
    api.ReadProject.mockReturnValueOnce(new Promise((r) => (resolve = r)));
    let switching!: Promise<boolean | undefined>;
    await act(async () => {
      switching = c.selectNamespace(a.key, "team", { uid: "one", valid: () => true });
      c.commit({ type: "focus", key: b.key });
    });
    const project = { resource: { uid: "one" }, phase: "Active" };
    await act(async () => {
      resolve(project);
      expect(await switching).toBe(false);
    });
    expect(api.SetWorkloadNamespace).not.toHaveBeenCalled();
    expect(c.state.tabs[b.key].projectUID).toBeUndefined();
    await act(async () => c.commit({ type: "focus", key: a.key }));
    api.ReadProject.mockResolvedValue(project);
    api.SetWorkloadNamespace.mockResolvedValue(a.view);
    await act(async () => {
      expect(await c.selectNamespace(a.key, "team", { uid: "one", valid: () => true })).toBe(true);
    });
    expect(c.state.tabs[a.key].projectUID).toBe("one");
    expect(api.ReadProject).toHaveBeenLastCalledWith(expect.any(String), a.view.ref, "team", "one");
    api.SetWorkloadNamespace.mockRejectedValue(new Error("transition failed"));
    await act(async () => {
      expect(await c.selectNamespace(a.key, "other", { uid: "", valid: () => true })).toBe(false);
    });
    expect(c.state.tabs[a.key].namespace).toBe("team");
    expect(c.state.tabs[a.key].projectUID).toBe("one");
  } finally {
    await act(async () => root.unmount());
    vi.unstubAllGlobals();
  }
});
