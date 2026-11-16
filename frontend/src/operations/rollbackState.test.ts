/*
Purpose: Verify that rollback confirmation remains bound to its selected revision.
Used by: The existing operation state tests.
Responsibilities: Exercise late preparation, stale captures and result wording.
Important: No simulated request can submit a real operation.
*/
import { expect, it, vi } from "vitest";
import { reviewOwner } from "./reviewState";
import { canConfirmRollback, rollbackAcceptedText, rollbackSelectionKey } from "./rollbackState";
import type { RollbackRequest } from "./api";

it("revokes late reviews and only confirms the original workload and revision capture", async () => {
  const connection = { id: "a", generation: 1 };
  const resource = { connection, apiVersion: "apps/v1", kind: "StatefulSet", name: "web", namespace: "default", uid: "web" };
  const a: RollbackRequest = {
    resource,
    resourceVersion: "7",
    source: { ...resource, kind: "ControllerRevision", name: "web-old", uid: "old" },
    sourceVersion: "10",
    revision: "9",
  };
  const b = { ...a, source: { ...a.source, name: "web-new", uid: "new" } };
  const revoke = vi.fn();
  const show = vi.fn();
  const owner = reviewOwner(revoke);
  let resolveA!: (review: { token: string }) => void;
  const promise = new Promise<{ token: string }>((resolve) => (resolveA = resolve));
  owner.prepare({ promise, cancel: vi.fn() }, show, vi.fn());
  owner.abandon();
  const preview = {
    request: b,
    token: "token-b",
    expiresAt: new Date(60000).toISOString(),
    notices: ["OnDelete: existing Pods will not be replaced automatically."],
  };
  owner.prepare({ promise: Promise.resolve(preview), cancel: vi.fn() }, show, vi.fn());
  resolveA({ token: "token-a" });
  await Promise.resolve();
  expect(show).toHaveBeenCalledExactlyOnceWith(preview);
  expect(revoke).toHaveBeenCalledWith("token-a");
  expect(rollbackSelectionKey(a)).not.toBe(rollbackSelectionKey(b));
  expect(canConfirmRollback(preview, b, 0)).toBe(true);
  expect(canConfirmRollback(preview, { ...b, resourceVersion: "8" }, 0)).toBe(false);
  expect(canConfirmRollback(preview, { ...b, sourceVersion: "11" }, 0)).toBe(false);
  expect(canConfirmRollback(preview, { ...b, resource: { ...resource, connection: { ...connection, id: "b" } } }, 0)).toBe(false);
  expect(canConfirmRollback(undefined, b, 0)).toBe(false);
  expect(canConfirmRollback(preview, b, 60000)).toBe(false);
  expect(owner.claim(preview.token)).toBe(true);
  expect(owner.claim(preview.token)).toBe(false);
  expect(rollbackAcceptedText).toContain("Pod replacement and readiness are separate");
});
