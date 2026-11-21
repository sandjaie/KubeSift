// @vitest-environment happy-dom
/*
Purpose: Verify on-demand historical chart ownership.
Used by: Frontend regression checks.
Responsibilities: Exercise range changes, collapse and stale responses.
Important: Deferred bridge responses are synthetic.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, test, vi } from "vitest";
import type { HistoricalMetrics } from "../../bindings/internal/kube/models";
import { HistoricalUsage } from "./HistoricalUsage";
const api = vi.hoisted(() => ({
  SetHistorySource: vi.fn(),
  HistorySource: vi.fn(),
  ReadHistoricalMetrics: vi.fn(),
  CancelWorkloadRead: vi.fn(async () => {}),
}));
vi.mock("../../bindings/internal/desktop", () => ({ ExplorerService: api }));
vi.mock("./api", () => ({
  onVisibility: (fn: (v: boolean) => void) => {
    fn(true);
    return () => {};
  },
}));
test("loads once on opening and cancels on range change and collapse", async () => {
  const host = document.createElement("div"),
    root = createRoot(host);
  api.HistorySource.mockResolvedValue({ origin: "https://fixture.test", revision: "r1", mode: "cluster", cluster: "a" });
  api.ReadHistoricalMetrics.mockImplementation(() => new Promise(() => {}));
  const resource = { connection: { id: "a", generation: 1 }, kind: "Pod", apiVersion: "v1", namespace: "test", name: "pod", uid: "uid" };
  await act(async () => root.render(<HistoricalUsage resource={resource} connection="A" containers={["app", "sidecar"]} />));
  expect(api.ReadHistoricalMetrics).not.toHaveBeenCalled();
  const toggle = host.querySelector("button")!;
  await act(async () => toggle.click());
  expect(api.ReadHistoricalMetrics.mock.calls[0]?.[1]).toMatchObject({ preset: "1h", container: "app", sourceID: "r1" });
  const range = host.querySelector<HTMLSelectElement>('select[aria-label="History range"]')!;
  await act(async () => {
    range.value = "6h";
    range.dispatchEvent(new Event("change", { bubbles: true }));
  });
  expect(api.CancelWorkloadRead).toHaveBeenCalledTimes(1);
  expect(api.ReadHistoricalMetrics.mock.calls[1]?.[1].preset).toBe("6h");
  await act(async () => toggle.click());
  expect(api.CancelWorkloadRead).toHaveBeenCalledTimes(2);
  await act(async () => root.unmount());
});

test("clears late chart results on container, replacement and identity changes", async () => {
  const host = document.createElement("div"),
    root = createRoot(host);
  const pending: { resolve: (v: HistoricalMetrics) => void; reject: (e: Error) => void }[] = [];
  api.ReadHistoricalMetrics.mockImplementation(() => new Promise((resolve, reject) => pending.push({ resolve, reject })));
  const source = { origin: "https://fixture.test", revision: "r1", mode: "cluster", cluster: "a" };
  api.HistorySource.mockResolvedValue(source);
  const resource = { connection: { id: "a", generation: 1 }, kind: "Pod", apiVersion: "v1", namespace: "test", name: "pod", uid: "uid" };
  const result = {
    sourceID: "r1",
    resource,
    container: "app",
    start: 0,
    end: 120,
    effectiveStart: 0,
    step: 30,
    cpuWindow: 300,
    fetchedAt: 120,
    cpu: { state: "available", qualified: true, points: [{ time: 0, value: 0 }] },
    memory: { state: "missing", qualified: false, points: null },
  };
  const render = async (id: string) =>
    act(async () =>
      root.render(
        <HistoricalUsage resource={{ ...resource, connection: { id, generation: 1 } }} connection={id} containers={["app", "sidecar"]} />,
      ),
    );
  const click = async (label: string) =>
    act(async () => {
      const b = [...host.querySelectorAll("button")].find((b) => b.textContent === label);
      expect(b).toBeDefined();
      b!.click();
    });
  await render("a");
  await click("Historical usage");
  await act(async () => pending[0].resolve(result));
  for (const word of ["0 mCPU", "Stale trailing coverage", "Gap", "partial", "No matching samples"])
    expect(host.textContent).toContain(word);
  await click("Refresh history");
  expect(host.textContent).not.toContain("0 mCPU");
  const container = host.querySelector("select")!;
  await act(async () => {
    container.value = "sidecar";
    container.dispatchEvent(new Event("change", { bubbles: true }));
  });
  await act(async () => pending[1].resolve(result));
  expect(host.textContent).not.toContain("0 mCPU");
  await act(async () => pending[2].reject(new Error("Provider denied")));
  expect(host.textContent).toContain("Provider denied");
  await click("Replace source");
  expect(host.textContent).toContain("Confirm source");
  expect(host.textContent).not.toContain("Provider denied");
  api.HistorySource.mockResolvedValue({ ...source, origin: "", revision: "" });
  await render("b");
  expect(host.textContent).toContain("No history source configured");
  await act(async () => root.unmount());
});
