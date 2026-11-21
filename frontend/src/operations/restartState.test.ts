/*
Purpose: Verify restart gates before showing or claiming a review.
Used by: Frontend tests.
Responsibilities: Exercise captured strategies, malformed data and exact acknowledgement.
Important: Native preparation and confirmation independently check eligibility.
*/
import { expect, test } from "vitest";
import { canConfirmRestart, restartUnavailable } from "./restartState";
import type { ResourceDetail } from "../workloads/model";

const source = (kind: string, spec: object, metadata: object = {}) =>
  ({
    resource: { kind, apiVersion: "apps/v1", uid: "uid" },
    resourceVersion: "7",
    objectJSON: JSON.stringify({ metadata, spec: { template: { metadata: {}, spec: {} }, ...spec } }),
  }) as ResourceDetail;

test("only captured eligible strategies can open restart", () => {
  expect(restartUnavailable(source("Deployment", {}))).toBe("");
  expect(restartUnavailable(source("Deployment", { strategy: { type: "Recreate" } }))).toBe("");
  expect(restartUnavailable(source("Deployment", { paused: true }))).toMatch(/paused/);
  expect(restartUnavailable(source("StatefulSet", { updateStrategy: { type: "OnDelete" } }))).toMatch(/OnDelete/);
  expect(restartUnavailable(source("DaemonSet", { updateStrategy: { type: "OnDelete" } }))).toMatch(/OnDelete/);
  expect(restartUnavailable(source("StatefulSet", { updateStrategy: { rollingUpdate: { partition: 1 } } }))).toMatch(/partition/);
  expect(restartUnavailable(source("Deployment", {}, { deletionTimestamp: "now" }))).toMatch(/terminating/);
  expect(restartUnavailable(source("DaemonSet", { template: [] }))).toMatch(/malformed/);
});

test("Recreate acknowledgement is case-sensitive and expires", () => {
  const review = { token: "token", expiresAt: "2026-11-15T00:05:00Z", impact: { recreate: true }, resource: { name: "web" } };
  const before = Date.parse("2026-11-15T00:00:00Z");
  expect(canConfirmRestart(review, "Web", before)).toBe(false);
  expect(canConfirmRestart(review, "web", before)).toBe(true);
  expect(canConfirmRestart(review, "web", Date.parse(review.expiresAt))).toBe(false);
});
