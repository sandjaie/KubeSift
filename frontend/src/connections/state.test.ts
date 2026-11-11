/*
Purpose: Protect independent live tab identities and late-response rejection.
Used by: Vitest.
Responsibilities: Exercise cancellation, namespace epochs and stale access evidence.
Important: Synthetic DTOs exercise the reducer without desktop or network access.
*/
import { expect, it } from "vitest";
import { initialConnections, reduceConnections, tabKey, visibleState } from "./state";
import type { CatalogEntry, ConnectionView } from "./api";
it("isolates connection attempts and namespaces", () => {
  const a: CatalogEntry = {
    sourceId: "a",
    snapshotId: "sa",
    context: "same",
    namespace: "alpha",
    endpoint: "https://a.invalid",
    sourceDisplay: "a",
    environment: "",
    labelNeedsReview: false,
  };
  const b = { ...a, sourceId: "b", snapshotId: "sb", namespace: "beta" };
  let s = initialConnections();
  s = reduceConnections(s, { type: "open", entry: a, request: 1 });
  s = reduceConnections(s, { type: "open", entry: b, request: 2 });
  expect(Object.keys(s.tabs)).toHaveLength(2);
  s = reduceConnections(s, { type: "attempt", key: tabKey(a), request: 1, id: "attempt-a" });
  const view: ConnectionView = {
    sourceId: "a",
    context: "same",
    attemptId: "attempt-a",
    namespace: "alpha",
    state: "connected",
    ref: { id: "ref-a", generation: 1 },
    problem: null,
    lastSuccessAt: "2026-11-06T00:00:00Z",
    namespaces: ["alpha"],
    namespacesComplete: true,
    pods: [],
    podProblem: null,
    podLoading: false,
  };
  s = reduceConnections(s, { type: "snapshot", key: tabKey(a), epoch: 0, view });
  expect(s.tabs[tabKey(a)].view?.ref?.id).toBe("ref-a");
  s = reduceConnections(s, { type: "filter", key: tabKey(a), filter: "same" });
  s = reduceConnections(s, { type: "select", key: tabKey(a), uid: "uid-a" });
  s = reduceConnections(s, { type: "focus", key: null });
  s = reduceConnections(s, { type: "focus", key: tabKey(b) });
  expect(s.tabs[tabKey(a)].filter).toBe("same");
  expect(s.tabs[tabKey(a)].selected).toBe("uid-a");
  expect(s.tabs[tabKey(b)].selected).toBeNull();
  s = reduceConnections(s, { type: "snapshot", key: tabKey(a), epoch: 0, view });
  expect(s.tabs[tabKey(a)].selected).toBeNull();
  s = reduceConnections(s, { type: "namespace", key: tabKey(a), namespace: "new" });
  s = reduceConnections(s, { type: "snapshot", key: tabKey(a), epoch: 0, view });
  expect(s.tabs[tabKey(a)].namespace).toBe("new");
  expect(s.tabs[tabKey(b)].namespace).toBe("beta");
  expect(visibleState(view, Date.parse("2026-11-06T00:02:00Z"))).toBe("stale");
  s = reduceConnections(s, { type: "close", key: tabKey(a) });
  s = reduceConnections(s, { type: "snapshot", key: tabKey(a), epoch: 0, view });
  expect(s.tabs[tabKey(a)]).toBeUndefined();
});

it("refreshes open tab labels without losing namespace or filter", () => {
  const entry: CatalogEntry = {
    sourceId: "a",
    snapshotId: "old",
    context: "same",
    namespace: "default",
    endpoint: "https://a.invalid",
    sourceDisplay: "a",
    environment: "Production",
    labelNeedsReview: false,
  };
  let s = reduceConnections(initialConnections(), { type: "open", entry, request: 1 });
  const key = tabKey(entry);
  s = reduceConnections(s, { type: "filter", key, filter: "keep" });
  const changed = { ...entry, snapshotId: "fresh", endpoint: "https://b.invalid", environment: "", labelNeedsReview: true };
  s = reduceConnections(s, {
    type: "sources",
    sources: [{ sourceId: "a", snapshotId: "fresh", path: "a", enabled: true, problem: null, contexts: [changed] }],
  });
  expect(s.tabs[key].entry.environment).toBe("");
  expect(s.tabs[key].entry.labelNeedsReview).toBe(true);
  expect(s.tabs[key].filter).toBe("keep");
  expect(s.tabs[key].namespace).toBe("default");
});
