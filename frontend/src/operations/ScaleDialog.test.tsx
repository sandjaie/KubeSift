/*
Purpose: Check visible scaling warnings and result claims.
Used by: Frontend regression tests.
Responsibilities: Render immutable scope, storage effects and unknown or admitted counts.
Important: Tests render inert evidence without a live bridge.
*/
import { test, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ScaleImpact } from "./ScaleDialog";
import { OperationResults } from "./OperationResults";
import type { ScalePreview, ScaleResult } from "./api";

const resource = {
  kind: "StatefulSet",
  apiVersion: "apps/v1",
  connection: { id: "c", generation: 2 },
  namespace: "fixture",
  name: "worker",
  uid: "id",
};
test("StatefulSet scale-down names the retention policy and does not promise availability", () => {
  const review: ScalePreview = {
    cluster: "local",
    resource,
    token: "review",
    sourceVersion: "7",
    before: 1,
    after: 0,
    expiresAt: "",
    permission: "unknown",
    warnings: [],
    impact: { direction: "zero", whenScaled: "Delete", whenDeleted: "Retain", policyDefault: false },
    noChange: false,
  };
  const text = renderToStaticMarkup(<ScaleImpact review={review} />);
  for (const phrase of ["Pods may terminate", "PodDisruptionBudgets", "when scaled: Delete", "When deleted: Retain", "Autoscalers"])
    expect(text).toContain(phrase);
});
test("result feedback labels uncertain outcomes and preserves requested and reported counts", () => {
  const baseline = { cluster: "local", resource, before: 1, requested: 2, startedAt: "", finishedAt: null, problem: null };
  const results: ScaleResult[] = [
    { ...baseline, operationID: "1", reported: null, state: "unknown" },
    { ...baseline, operationID: "2", reported: 3, state: "accepted" },
  ];
  const text = renderToStaticMarkup(
    <OperationResults
      results={results}
      pending={0}
      error=""
      onClose={() => {}}
      onRefresh={() => {}}
      onTarget={() => {}}
      canOpen={() => true}
    />,
  );
  for (const phrase of [
    "Outcome unknown",
    "local / fixture / StatefulSet / worker",
    "Requested: 2. API reported: 3.",
    "Accepted by Kubernetes",
    "Pod readiness",
  ])
    expect(text).toContain(phrase);
});
