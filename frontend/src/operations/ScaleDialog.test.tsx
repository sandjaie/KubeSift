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
import type { OperationResult, ScalePreview } from "./api";

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
  const baseline = { cluster: "local", resource, startedAt: "", finishedAt: null, problem: null };
  const counts = { before: 1, requested: 2 };
  const results: OperationResult[] = [
    { ...baseline, action: "scale", operationID: "1", scale: { ...counts, reported: null }, restart: null, state: "unknown" },
    { ...baseline, action: "scale", operationID: "2", scale: { ...counts, reported: 3 }, restart: null, state: "accepted" },
    {
      ...baseline,
      action: "restart",
      operationID: "3",
      scale: null,
      restart: { requestedAt: "2026-11-15T00:00:00Z", responseDiffers: true },
      state: "accepted",
    },
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
    "Restart · Accepted by Kubernetes",
    "2026-11-15T00:00:00Z",
    "server response differs from the reviewed spec",
  ])
    expect(text).toContain(phrase);
});
