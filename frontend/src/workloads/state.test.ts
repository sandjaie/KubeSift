/*
Purpose: Protect independent workload navigation and stale-response rejection.
Used by: Vitest.
Responsibilities: Exercise A/B selection, generation invalidation and bounded pagination.
Important: No native bridge or cluster is used.
*/
import { expect, test } from "vitest";
import { initialWorkloads, reduceWorkloads, emptyKind, canAutoRefresh } from "./state";

test("A/B kinds retain selection, reject delayed responses and bound cursor history", () => {
  let state = initialWorkloads();
  state = reduceWorkloads(state, { type: "scope", tab: "A", scope: "a/1/default" });
  state = reduceWorkloads(state, { type: "scope", tab: "B", scope: "b/1/default" });
  state = reduceWorkloads(state, { type: "kind", tab: "B", kind: "Deployment" });
  state = reduceWorkloads(state, { type: "patch", tab: "A", scope: "a/1/default", kind: "Pod", patch: { filter: "crash", epoch: 2 } });
  state = reduceWorkloads(state, { type: "response", tab: "A", scope: "a/1/default", kind: "Pod", epoch: 1, patch: { filter: "wrong" } });
  expect(state.A.kinds.Pod.filter).toBe("crash");
  expect(state.B.kind).toBe("Deployment");
  state = reduceWorkloads(state, { type: "scope", tab: "A", scope: "a/2/other" });
  state = reduceWorkloads(state, { type: "response", tab: "A", scope: "a/1/default", kind: "Pod", epoch: 2, patch: { filter: "wrong" } });
  expect(state.A.kinds.Pod.filter).toBe("");
  for (let n = 0; n < 25; n++) state = reduceWorkloads(state, { type: "next", tab: "A", cursor: `page-${n}` });
  expect(state.A.kinds.Pod.history).toHaveLength(20);
  expect(canAutoRefresh({ ...emptyKind(), auto: true }, true)).toBe(true);
  expect(canAutoRefresh({ ...state.A.kinds.Pod, auto: true }, true)).toBe(false);
  expect(canAutoRefresh({ ...emptyKind(), auto: true }, false)).toBe(false);
});

test("reselecting the same resource or section preserves pending evidence", () => {
  let s = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "a/1/ns" });
  const row = {
    resource: { connection: { id: "a", generation: 1 }, apiVersion: "v1", kind: "Pod", namespace: "ns", name: "pod", uid: "uid" },
    phase: "Running",
    reason: "",
    node: "",
    ready: 1,
    desired: 1,
    restarts: 0,
    available: null,
    updated: null,
    current: null,
    total: null,
    job: null,
    cronJob: null,
    createdAt: null,
  };
  s = reduceWorkloads(s, { type: "select", tab: "A", row, epoch: 1 });
  expect(s.A.kinds.Pod.selected?.resource.uid).toBe("uid");
  s = reduceWorkloads(s, {
    type: "patch",
    tab: "A",
    scope: "a/1/ns",
    kind: "Pod",
    patch: { sections: { Overview: { loading: true, error: "", data: null } }, sectionEpoch: 2 },
  });
  s = reduceWorkloads(s, { type: "select", tab: "A", row, epoch: 3 });
  s = reduceWorkloads(s, { type: "section", tab: "A", section: "Overview", epoch: 4 });
  expect(s.A.kinds.Pod.sectionEpoch).toBe(2);
  expect(s.A.kinds.Pod.sections.Overview?.loading).toBe(true);
});

test("observed Deployment zero counts remain zero while sparse status stays unknown", async () => {
  const { deploymentCounts } = await import("./model");
  expect(deploymentCounts({ observedGeneration: 1 })).toEqual({ updated: 0, ready: 0, available: 0, unavailable: 0 });
  expect(deploymentCounts({})).toEqual({ updated: undefined, ready: undefined, available: undefined, unavailable: undefined });
});

test("namespace reconciliation retains workload kind and auto-refresh but invalidates evidence", () => {
  let state = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "old" });
  state = reduceWorkloads(state, { type: "kind", tab: "A", kind: "Deployment" });
  state = reduceWorkloads(state, {
    type: "patch",
    tab: "A",
    scope: "old",
    kind: "Deployment",
    patch: { auto: true, filter: "old", epoch: 3, scroll: 50, history: ["", "next"] },
  });
  const next = reduceWorkloads(state, { type: "scope", tab: "A", scope: "new" });
  expect(next.A.kind).toBe("Deployment");
  expect(next.A.kinds.Deployment).toEqual({ ...emptyKind(), auto: true });
  expect(
    reduceWorkloads(next, { type: "response", tab: "A", scope: "old", kind: "Deployment", epoch: 3, patch: { error: "forbidden" } }),
  ).toBe(next);
  expect(reduceWorkloads(next, { type: "scope", tab: "A", scope: "new" })).toBe(next);
});

test("relationship return restores the originating section and rejects a stale scope", () => {
  let state = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "scope" });
  const resource = {
    connection: { id: "a", generation: 1 },
    apiVersion: "apps/v1",
    kind: "ReplicaSet",
    namespace: "ns",
    name: "web",
    uid: "set",
  };
  const entry = { resource, section: "Pods" as const, cursor: "related-page" };
  state = reduceWorkloads(state, { type: "navigate", tab: "A", scope: "scope", kind: "ReplicaSet", entry, history: [], epoch: 4 });
  expect(state.A.kind).toBe("ReplicaSet");
  expect(state.A.kinds.ReplicaSet).toMatchObject({ selected: { resource }, section: "Pods", sectionCursor: "related-page", related: true });
  const next = reduceWorkloads(state, { type: "scope", tab: "A", scope: "other" });
  expect(next.A).toMatchObject({ kind: "ReplicaSet", navigation: [], kinds: { ReplicaSet: { selected: null, sections: {} } } });
  expect(reduceWorkloads(next, { type: "navigate", tab: "A", scope: "scope", kind: "ReplicaSet", entry, history: [], epoch: 4 })).toBe(
    next,
  );
});

test("CronJob to Job to Pod restores the originating sections and clears old scope", () => {
  let state = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "a" });
  const resource = { connection: { id: "a", generation: 1 }, apiVersion: "batch/v1", namespace: "test", name: "sample" };
  const cron = { resource: { ...resource, kind: "CronJob", uid: "cron" }, section: "Jobs", cursor: "jobs-page" } as const;
  const job = { resource: { ...resource, kind: "Job", uid: "job" }, section: "Pods", cursor: "pods-page" } as const;
  const pod = { resource: { ...resource, apiVersion: "v1", kind: "Pod", uid: "pod" }, section: "Overview", cursor: "" } as const;
  state = reduceWorkloads(state, { type: "navigate", tab: "A", scope: "a", kind: "Pod", entry: pod, history: [cron, job], epoch: 1 });
  state = reduceWorkloads(state, { type: "navigate", tab: "A", scope: "a", kind: "Job", entry: job, history: [cron], epoch: 2 });
  expect(state.A.kinds.Job).toMatchObject({ section: "Pods", sectionCursor: "pods-page" });
  state = reduceWorkloads(state, { type: "navigate", tab: "A", scope: "a", kind: "CronJob", entry: cron, history: [], epoch: 3 });
  expect(state.A.kinds.CronJob).toMatchObject({ section: "Jobs", sectionCursor: "jobs-page" });
  state = reduceWorkloads(state, { type: "scope", tab: "A", scope: "b" });
  expect(state.A.kind).toBe("CronJob");
  expect(state.A.kinds.CronJob.selected).toBeNull();
  expect(state.A.navigation).toEqual([]);
  expect(reduceWorkloads(state, { type: "navigate", tab: "A", scope: "a", kind: "Job", entry: job, history: [cron], epoch: 4 })).toBe(
    state,
  );
});

test("IngressClass scope ignores namespace changes but invalidates on a new connection generation", () => {
  let state = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "a/1/ns", clusterScope: "a/1" });
  state = reduceWorkloads(state, { type: "kind", tab: "A", kind: "IngressClass" });
  state = reduceWorkloads(state, { type: "patch", tab: "A", scope: "a/1", kind: "IngressClass", patch: { filter: "class", epoch: 4 } });
  state = reduceWorkloads(state, { type: "scope", tab: "A", scope: "a/1/other", clusterScope: "a/1" });
  expect(state.A.kinds.IngressClass.filter).toBe("class");
  state = reduceWorkloads(state, {
    type: "response",
    tab: "A",
    scope: "a/1",
    kind: "IngressClass",
    epoch: 4,
    patch: { error: "class unavailable" },
  });
  expect(state.A.kinds.IngressClass.error).toBe("class unavailable");
  state = reduceWorkloads(state, { type: "scope", tab: "A", scope: "a/2/other", clusterScope: "a/2" });
  expect(state.A.kind).toBe("IngressClass");
  expect(state.A.kinds.IngressClass).toEqual(emptyKind());
});

test("networking Back restores section, position and identity without requiring inventory membership", () => {
  let state = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "a" });
  const ref = { connection: { id: "a", generation: 1 }, namespace: "ns", name: "sample", apiVersion: "v1" };
  const ingress = { resource: { ...ref, kind: "Ingress", uid: "ing" }, section: "Overview", cursor: "" } as const;
  const service = {
    resource: { ...ref, kind: "Service", uid: "svc" },
    section: "EndpointSlices",
    cursor: "slices",
    position: { filter: "web", history: ["", "page2"], scroll: 50, sort: "desc" },
  } as const;
  const slice = { resource: { ...ref, kind: "EndpointSlice", uid: "slice" }, section: "Overview", cursor: "" } as const;
  const pod = { resource: { ...ref, kind: "Pod", uid: "pod" }, section: "Logs", cursor: "", identity: "Referenced UID verified" } as const;
  state = reduceWorkloads(state, {
    type: "navigate",
    tab: "A",
    scope: "a",
    kind: "Pod",
    entry: pod,
    history: [ingress, { ...service, position: { ...service.position, history: ["", "page2"] } }, slice],
    epoch: 1,
  });
  expect(state.A.kinds.Pod).toMatchObject({ section: "Logs", related: true, identity: "Referenced UID verified" });
  state = reduceWorkloads(state, {
    type: "navigate",
    tab: "A",
    scope: "a",
    kind: "Service",
    entry: { ...service, position: { ...service.position, history: ["", "page2"] } },
    history: [ingress],
    epoch: 2,
  });
  expect(state.A.kinds.Service).toMatchObject({
    section: "EndpointSlices",
    sectionCursor: "slices",
    related: true,
    filter: "web",
    scroll: 50,
    history: ["", "page2"],
  });
});

test("selecting the current related row preserves an in-flight Overview when clearing Back", () => {
  const resource = { connection: { id: "a", generation: 1 }, apiVersion: "v1", kind: "Service", namespace: "ns", name: "web", uid: "web" };
  let state = reduceWorkloads(initialWorkloads(), { type: "scope", tab: "A", scope: "a" });
  state = reduceWorkloads(state, {
    type: "navigate",
    tab: "A",
    scope: "a",
    kind: "Service",
    entry: { resource, section: "Overview", cursor: "" },
    history: [],
    epoch: 1,
  });
  state = reduceWorkloads(state, {
    type: "patch",
    tab: "A",
    scope: "a",
    kind: "Service",
    patch: { sections: { Overview: { loading: true, error: "", data: null } } },
  });
  state = reduceWorkloads(state, { type: "select", tab: "A", row: { resource }, epoch: 2 });
  expect(state.A.kinds.Service).toMatchObject({ related: false, sectionEpoch: 1, sections: { Overview: { loading: true } } });
});
