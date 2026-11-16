/*
Purpose: Check that a late review cannot confirm another edit.
Used by: Editor state tests.
Responsibilities: Exercise target identity, review invalidation and exact diff text.
Important: Only synthetic YAML appears in this test.
*/
import { expect, test, vi } from "vitest";
import { canConfirmEdit, editDiff, sameEditTarget } from "./editState";
import { reviewOwner } from "./reviewState";

test("a changed draft revokes a late preview and retains the original resource boundary", async () => {
  const target = {
    connection: { id: "A", generation: 1 },
    group: "",
    version: "v1",
    resource: "secrets",
    kind: "Secret",
    namespace: "demo",
    name: "sample",
    uid: "uid",
  };
  const other = { ...target, connection: { id: "B", generation: 1 } };
  const preview = { token: "prepared", target, expiresAt: "2026-11-17T00:00:00Z" };
  const revoke = vi.fn();
  const cancel = vi.fn();
  const show = vi.fn();
  const owner = reviewOwner(revoke);
  let finish!: (value: typeof preview) => void;
  owner.prepare(
    {
      promise: new Promise((resolve) => {
        finish = resolve;
      }),
      cancel,
    },
    show,
    vi.fn(),
  );
  owner.abandon();
  finish(preview);
  await Promise.resolve();
  expect(cancel).toHaveBeenCalledOnce();
  expect(revoke).toHaveBeenCalledWith("prepared");
  expect(show).not.toHaveBeenCalled();
  expect(sameEditTarget(target, other)).toBe(false);
  expect(canConfirmEdit(preview, target, 0)).toBe(true);
  expect(canConfirmEdit(preview, other, 0)).toBe(false);
  expect(editDiff("data:\n  one: old\n  two: kept\n", "data:\n  one: new\n  two: kept\n")).toContain("+   one: new");
});

test("a large YAML review still shows both changed values", () => {
  const rows = Array.from({ length: 800 }, (_, index) => `  row${index}: kept`);
  const before = ["data:", ...rows, "  marker: old", ...rows].join("\n");
  const after = before.replace("marker: old", "marker: new");
  const diff = editDiff(before, after);
  expect(diff).toContain("-   marker: old");
  expect(diff).toContain("+   marker: new");
  expect(diff).not.toContain("row0");
});
