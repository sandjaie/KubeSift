/*
Purpose: Check exact acknowledgement and captured deletion eligibility.
Used by: Frontend operation regression tests.
Responsibilities: Reject expired reviews, unsafe claim policies and mirror or terminating workloads.
Important: Native eligibility remains authoritative.
*/
import { expect, test } from "vitest";
import type { ResourceDetail } from "../workloads/model";
import { canConfirmDelete, deleteUnavailable } from "./deleteState";

test("confirmation requires the exact name and a live review", () => {
  const review = { token: "review", expiresAt: "2026-11-16T12:00:00Z", resource: { name: "web" } };
  const expiry = Date.parse(review.expiresAt);
  expect(canConfirmDelete(review, "web", expiry - 1)).toBe(true);
  expect(canConfirmDelete(review, "web", expiry)).toBe(false);
  expect(canConfirmDelete(review, "Web", expiry - 1)).toBe(false);
  expect(canConfirmDelete(review, "web ", expiry - 1)).toBe(false);
});

test("captured evidence explains StatefulSet, mirror and terminating restrictions", () => {
  const source = {
    resource: { kind: "StatefulSet", uid: "uid", namespace: "fixture" },
    resourceVersion: "7",
    objectJSON: '{"spec":{}}',
  } as ResourceDetail;
  expect(deleteUnavailable(source)).toBe("");
  expect(
    deleteUnavailable({ ...source, objectJSON: '{"spec":{"persistentVolumeClaimRetentionPolicy":{"whenDeleted":"Delete"}}}' }),
  ).toContain("Retain");
  expect(deleteUnavailable({ ...source, deleteBlocked: "Mirror Pods are managed by the node." })).toContain("Mirror Pods");
  expect(deleteUnavailable({ ...source, objectJSON: '{"metadata":{"deletionTimestamp":"now"}}' })).toContain("terminating");
});
