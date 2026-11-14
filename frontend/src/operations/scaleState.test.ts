/*
Purpose: Verify scaling draft input and review ownership.
Used by: Frontend regression tests.
Responsibilities: Keep decimal input exact and reject expired or obsolete confirmations.
Important: No native write is issued by these tests.
*/
import { expect, test } from "vitest";
import { validReplicaText, canConfirmScale } from "./scaleState";

test("scale input accepts exact ASCII decimal values in the API range", () => {
  for (const value of ["0", "0001", "2147483647"]) expect(validReplicaText(value)).toBe(true);
  for (const value of ["", "2e1", " 2", "-1", "1.1", "٢", "2147483648"]) expect(validReplicaText(value)).toBe(false);
});
test("confirm requires a live review for the current draft and exact acknowledgement at zero", () => {
  const preview = { noChange: false, token: "t", after: 0, expiresAt: "2026-11-15T00:00:00Z", resource: { name: "workload" } };
  const now = Date.parse("2026-11-14T23:59:59Z");
  expect(canConfirmScale(preview, "0", "workload", now)).toBe(true);
  expect(canConfirmScale(preview, "0", "Workload", now)).toBe(false);
  expect(canConfirmScale(preview, "1", "workload", now)).toBe(false);
  expect(canConfirmScale(preview, "0", "workload", now + 1000)).toBe(false);
});

test("departing cancels an active preparation and revokes a late token", async () => {
  const { scaleReviewOwner } = await import("./scaleState");
  const revoked: string[] = [];
  const owner = scaleReviewOwner((token) => revoked.push(token));
  let resolve!: (review: { token: string }) => void;
  let cancelled = 0;
  const visible: string[] = [];
  owner.prepare(
    {
      promise: new Promise<{ token: string }>((done) => {
        resolve = done;
      }),
      cancel: () => {
        cancelled++;
      },
    },
    (review) => visible.push(review.token),
    () => {},
  );
  owner.abandon();
  resolve({ token: "late" });
  await Promise.resolve();
  expect(cancelled).toBe(1);
  expect(revoked).toEqual(["late"]);
  expect(visible).toEqual([]);
});

test("a claimed token cannot be submitted twice or revoked on dialog unmount", async () => {
  const { scaleReviewOwner } = await import("./scaleState");
  const revoked: string[] = [];
  const owner = scaleReviewOwner((token) => revoked.push(token));
  owner.prepare(
    { promise: Promise.resolve({ token: "reviewed" }), cancel: () => {} },
    () => {},
    () => {},
  );
  await Promise.resolve();
  expect(owner.claim("reviewed")).toBe(true);
  expect(owner.claim("reviewed")).toBe(false);
  owner.abandon();
  expect(revoked).toEqual([]);
});
