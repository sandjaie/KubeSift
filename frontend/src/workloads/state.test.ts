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
