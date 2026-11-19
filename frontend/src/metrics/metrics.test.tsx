// @vitest-environment happy-dom
/*
Purpose: Check shared visible-page metrics polling and honest presentation.
Used by: Frontend regression checks.
Responsibilities: Exercise cancellation, late replies, freshness and retained evidence.
Important: Bridge responses are synthetic; packaged native behavior is checked separately.
*/
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { metricsIdentity, useMetrics } from "./useMetrics";
import { CurrentUsage, MetricsCell } from "./MetricsView";
import type { ResourceUsage, MetricsBatch, ResourceRef } from "../../bindings/internal/kube/models";
const api = vi.hoisted(() => ({
  readMetrics: vi.fn(),
  onVisibility: vi.fn<(listener: (visible: boolean) => void) => () => void>(() => () => {}),
}));
vi.mock("./api", () => api);
const ref: ResourceRef = {
  connection: { id: "a", generation: 1 },
  apiVersion: "v1",
  kind: "Pod",
  namespace: "test",
  name: "sample",
  uid: "uid",
};
const value = {
  amount: "0",
  request: null,
  limit: null,
  allocatable: null,
  requestPercent: null,
  limitPercent: null,
  allocatablePercent: null,
};
function sample(resource = ref): ResourceUsage {
  return {
    resource,
    apiVersion: "metrics.k8s.io/v1",
    state: "available",
    sampledAt: new Date().toISOString(),
    windowSeconds: "30",
    fetchedAt: new Date().toISOString(),
    configuredAt: new Date().toISOString(),
    cpu: value,
    memory: { ...value, amount: "83886080" },
    cpuComplete: true,
    memoryComplete: true,
    missingContainers: null,
    containers: [{ name: "app", category: "application", cpu: { ...value, request: "1", requestPercent: "200" }, memory: value }],
    podLevelBudget: false,
    problem: null,
  };
}
let host: HTMLDivElement, root: ReturnType<typeof createRoot>;
let pending: { resolve: (batch: MetricsBatch) => void; cancel: ReturnType<typeof vi.fn> }[];
function Harness({ target = ref, visible = true }: { target?: ResourceRef; visible?: boolean }) {
  const m = useMetrics({ connection: target.connection, targets: [target, target], detail: target, visible });
  return (
    <>
      <button onClick={m.refresh}>Refresh</button>
      <MetricsCell view={m.byIdentity[metricsIdentity(target)]} metric="cpu" />
      <CurrentUsage view={m.byIdentity[metricsIdentity(target)]} />
    </>
  );
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-19T10:00:00Z"));
  pending = [];
  api.readMetrics.mockImplementation(() => {
    const cancel = vi.fn();
    return { promise: new Promise<MetricsBatch>((resolve) => pending.push({ resolve, cancel })), cancel };
  });
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.useRealTimers();
  vi.clearAllMocks();
});
const render = async (target = ref, visible = true) => {
  await act(async () => root.render(<Harness target={target} visible={visible} />));
};
const reply = async (index: number, item = sample(), problem: MetricsBatch["problem"] = null) => {
  await act(async () => pending[index].resolve({ items: [item], fetchedAt: new Date().toISOString(), problem }));
};
test("deduplicates, waits after completion, cancels and isolates late scopes", async () => {
  await render();
  expect(api.readMetrics.mock.calls[0][0].targets).toHaveLength(1);
  await act(async () => vi.advanceTimersByTime(45000));
  expect(pending).toHaveLength(1);
  await reply(0);
  expect(host.textContent).toContain("0m");
  expect(host.textContent).toContain("80 MiB");
  expect(host.textContent).toContain("200% of request");
  await act(async () => vi.advanceTimersByTime(14999));
  expect(pending).toHaveLength(1);
  await act(async () => vi.advanceTimersByTime(1));
  expect(pending).toHaveLength(2);
  await render(ref, false);
  expect(pending[1].cancel).toHaveBeenCalled();
  const other = { ...ref, connection: { id: "b", generation: 1 } };
  await render(other);
  expect(pending).toHaveLength(3);
  await reply(1);
  expect(host.textContent).not.toContain("80 MiB");
  await reply(2, sample(other));
  await act(async () => vi.advanceTimersByTime(61000));
  expect(host.textContent).toContain("Stale");
});
test("retains transient failures, clears forbidden values and permits immediate refresh", async () => {
  await render();
  await reply(0);
  await act(async () => vi.advanceTimersByTime(15000));
  await reply(
    1,
    {
      ...sample(),
      state: "unavailable",
      cpu: { ...value, amount: null },
      memory: { ...value, amount: null },
      problem: { code: "unavailable", message: "Provider unavailable" },
    },
    { code: "unavailable", message: "Provider unavailable" },
  );
  expect(host.textContent).toContain("Last sample");
  await act(async () => vi.advanceTimersByTime(59000));
  expect(pending).toHaveLength(2);
  await act(async () => host.querySelector("button")!.click());
  expect(pending).toHaveLength(3);
  await reply(2, {
    ...sample(),
    state: "unavailable",
    cpu: { ...value, amount: null },
    memory: { ...value, amount: null },
    problem: { code: "forbidden", message: "Denied" },
  });
  expect(host.textContent).not.toContain("80 MiB");
  expect(host.textContent).toContain("Denied");
  expect(host.querySelector("button")!.disabled).toBe(false);
});

test("native hide/wake cancels current work and refreshes without backlog", async () => {
  await render();
  await reply(0);
  await act(async () => vi.advanceTimersByTime(15000));
  const listener = api.onVisibility.mock.calls[0][0];
  await act(async () => listener(false));
  expect(pending[1].cancel).toHaveBeenCalled();
  expect(host.textContent).toContain("80 MiB");
  await act(async () => vi.advanceTimersByTime(120000));
  expect(pending).toHaveLength(2);
  expect(host.textContent).toContain("Stale");
  await act(async () => listener(true));
  expect(pending).toHaveLength(3);
});

test("labels partial and Node comparisons and splits selected-first pages sequentially", async () => {
  const item = sample();
  item.cpuComplete = false;
  item.missingContainers = ["sidecar"];
  const view = { sample: item, now: Date.now() };
  await act(async () => root.render(<CurrentUsage view={view} />));
  expect(host.textContent).toContain("Partial subtotal");
  expect(host.textContent).toContain("Incomplete coverage: sidecar");
  const node = {
    ...item,
    resource: { ...ref, kind: "Node", namespace: "" },
    containers: null,
    cpu: { ...value, allocatable: "2", allocatablePercent: "150" },
  };
  await act(async () => root.render(<CurrentUsage view={{ sample: node, now: Date.now() }} />));
  expect(host.textContent).toContain("150% of allocatable");
  function Page() {
    useMetrics({
      connection: ref.connection,
      targets: Array.from({ length: 101 }, (_, i) => ({ ...ref, name: `pod-${i}`, uid: `${i}` })),
      detail: ref,
      visible: true,
    });
    return null;
  }
  await act(async () => root.render(<Page />));
  expect(api.readMetrics.mock.calls[0][0].targets).toHaveLength(100);
  expect(api.readMetrics.mock.calls[0][0].targets[0]).toEqual(ref);
  await reply(0);
  expect(api.readMetrics.mock.calls[1][0].targets).toHaveLength(2);
});
