// @vitest-environment happy-dom
/*
Purpose: Keep a confirmed Project selector tied to discovery for its original generation.
Used by: React discovery regressions.
Responsibilities: Exercise pending and failed refresh, tab return and confirmed absence.
Important: Project names never fall back to core Namespace suggestions.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import { useProjects } from "./useProjects";
const api = vi.hoisted(() => ({ ProjectCapabilities: vi.fn(), ListProjects: vi.fn() }));
vi.mock("../connections/api", () => ({ api, message: String }));
vi.mock("../workloads/api", () => ({
  readRequest: (read: (id: string) => Promise<unknown>) => ({ promise: read("fixture"), cancel: vi.fn() }),
}));
test("failed refresh preserves Projects mode and disables unverified choices", async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const root = createRoot(document.createElement("div"));
  const connection = { id: "A", generation: 1 };
  let result!: ReturnType<typeof useProjects>;
  function Harness() {
    result = useProjects(connection, [connection]);
    return null;
  }
  api.ProjectCapabilities.mockResolvedValueOnce({ projects: true, list: true, get: true });
  api.ListProjects.mockResolvedValueOnce({ items: [], partial: false, next: "" });
  try {
    await act(async () => root.render(<Harness />));
    expect(result.inventory?.caps?.get).toBe(true);
    let reject!: (e: Error) => void;
    api.ProjectCapabilities.mockReturnValueOnce(
      new Promise((_, r) => {
        reject = r;
      }),
    );
    await act(async () => result.refresh());
    expect(result.inventory?.caps).toMatchObject({ projects: true, get: false });
    expect(result.inventory?.items).toEqual([]);
    await act(async () => reject(new Error("forbidden")));
    expect(result.inventory?.caps?.projects).toBe(true);
    expect(result.inventory?.error).toContain("forbidden");
    api.ProjectCapabilities.mockResolvedValueOnce({ projects: false, list: false, get: false });
    await act(async () => result.refresh());
    expect(result.inventory?.caps?.projects).toBe(false);
  } finally {
    await act(async () => root.unmount());
    vi.unstubAllGlobals();
  }
});
