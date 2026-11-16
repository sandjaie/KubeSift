/*
Purpose: Verify deletion evidence and accepted result wording.
Used by: Frontend operation regression tests.
Responsibilities: Render precise impact and keep removal observations separate.
Important: The tests use synthetic native projections.
*/
import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { DeleteEvidence } from "./DeleteDialog";
import { OperationResults } from "./OperationResults";
import type { DeletePreview, OperationResult } from "./api";

const resource = {
  kind: "Pod",
  apiVersion: "v1",
  connection: { id: "c", generation: 2 },
  namespace: "fixture",
  name: "web",
  uid: "id",
};
const policy = { propagation: "Background", gracePolicy: "Server default" };

test("review exposes bounded owner and finalizer evidence and direct Pod impact", () => {
  const review: DeletePreview = {
    resource,
    sourceVersion: "7",
    cluster: "local",
    expiresAt: "2026-11-16T12:00:00Z",
    permission: "unknown",
    token: "review",
    warnings: [],
    impact: {
      ...policy,
      podGrace: 30,
      podGraceDefault: true,
      owner: { apiVersion: "apps/v1", kind: "ReplicaSet", name: "owner", uid: "owner-uid" },
      ownershipUnknown: false,
      finalizers: ["fixture.example/keep"],
      finalizerCount: 101,
      finalizersPartial: true,
      whenDeleted: "",
      retentionDefault: false,
      desired: null,
      current: null,
      ready: null,
      active: null,
    },
  };
  const text = renderToStaticMarkup(<DeleteEvidence review={review} />);
  for (const phrase of [
    "Background",
    "Server default",
    "30s (API default)",
    "owner-uid",
    "may recreate",
    "101 (showing 1)",
    "fixture.example/keep",
    "PodDisruptionBudget admission check",
    "Server dry-run passed",
  ])
    expect(text).toContain(phrase);
  const cron = renderToStaticMarkup(
    <DeleteEvidence
      review={{ ...review, resource: { ...resource, kind: "CronJob" }, impact: { ...review.impact, ownershipUnknown: true } }}
    />,
  );
  for (const phrase of ["Unknown", "stops future schedules", "retained Jobs and their Pods", "ownership is unknown"])
    expect(cron).toContain(phrase);
});

test("an accepted deletion offers an operation-bound check and keeps the original identity", () => {
  const result: OperationResult = {
    action: "delete",
    state: "accepted",
    delete: policy,
    restart: null,
    scale: null,
    problem: null,
    resource,
    cluster: "local",
    startedAt: "2026-11-16T12:00:00Z",
    finishedAt: "2026-11-16T12:00:01Z",
    operationID: "op",
  };
  const text = renderToStaticMarkup(
    <OperationResults
      results={[result]}
      pending={0}
      error=""
      onClose={() => {}}
      onRefresh={() => {}}
      onTarget={() => {}}
      canOpen={() => true}
    />,
  );
  for (const phrase of ["Delete · Accepted by Kubernetes", "local / fixture / Pod / web", "may still be present", "Check target"])
    expect(text).toContain(phrase);
  expect(text).not.toContain("Open target / refresh");
});
