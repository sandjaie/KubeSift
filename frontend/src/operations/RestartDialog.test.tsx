/*
Purpose: Check the visible restart review contract.
Used by: Frontend regression tests.
Responsibilities: Render the exact annotation, rollout impact and bounded evidence.
Important: Rendering never executes a native action.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { RestartEvidence } from "./RestartDialog";
import type { RestartPreview } from "./api";
import { reviewOwner } from "./reviewState";

const review: RestartPreview = {
  token: "test",
  cluster: "local",
  resource: {
    kind: "Deployment",
    apiVersion: "apps/v1",
    connection: { id: "c", generation: 2 },
    namespace: "fixture",
    name: "demo",
    uid: "uid",
  },
  sourceVersion: "7",
  expiresAt: "2026-11-15T00:05:00Z",
  restartAt: "2026-11-15T00:00:00Z",
  previousSet: true,
  previousValue: "<literal>",
  previousTruncated: true,
  warnings: ["Synthetic admission warning"],
  permission: "unknown",
  impact: {
    strategy: "Recreate",
    strategyDefault: false,
    desired: 0,
    desiredDefault: false,
    current: null,
    ready: null,
    maxSurge: "",
    maxSurgeDefault: false,
    maxUnavailable: "",
    maxUnavailableDefault: false,
    partition: null,
    partitionDefault: false,
    recreate: true,
    zeroPods: true,
  },
};

test("review distinguishes exact changes, observations and disruption warnings", () => {
  const html = renderToStaticMarkup(<RestartEvidence review={review} />);
  for (const expected of [
    "Recreate can terminate all existing Pods",
    "PodDisruptionBudgets",
    "External automation",
    "No current Pods are expected",
    ">Unknown<",
    "kubectl.kubernetes.io/restartedAt",
    "&lt;literal&gt;",
    "(truncated)",
    review.restartAt,
    "Server dry-run passed",
    "Review expires",
    "Synthetic admission warning",
  ])
    expect(html).toContain(expected);
});

test("abandon revokes late restart review while claimed token cannot be resubmitted", async () => {
  const revoked: string[] = [];
  const owner = reviewOwner((token) => revoked.push(token));
  let settle!: (value: RestartPreview) => void;
  let cancelled = false;
  let shown = false;
  owner.prepare(
    {
      promise: new Promise<RestartPreview>((resolve) => {
        settle = resolve;
      }),
      cancel: () => {
        cancelled = true;
      },
    },
    () => {
      shown = true;
    },
    () => {},
  );
  owner.abandon();
  settle(review);
  await Promise.resolve();
  expect([cancelled, shown, revoked]).toEqual([true, false, ["test"]]);
  owner.prepare(
    { promise: Promise.resolve(review), cancel: () => {} },
    () => {},
    () => {},
  );
  await Promise.resolve();
  expect(owner.claim("test")).toBe(true);
  expect(owner.claim("test")).toBe(false);
  owner.abandon();
  expect(revoked).toEqual(["test"]);
});
