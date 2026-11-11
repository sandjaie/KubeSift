/*
Purpose: Protect close/reopen ownership while Start RPC responses are deferred.
Used by: Vitest.
Responsibilities: Ensure old cancellation finishes before a new same-context Start can adopt an attempt.
Important: The fake boundary is the native RPC; the serialization used by the hook is real.
*/
import { expect, it } from "vitest";
import { createAttemptQueue } from "./attempts";
it("settles superseded Start cancellation before reopening the same context", async () => {
  const queue = createAttemptQueue();
  let release!: () => void;
  const delayed = new Promise<void>((resolve) => {
    release = resolve;
  });
  const events: string[] = [];
  const old = queue("a", async () => {
    events.push("start-old");
    await delayed;
    events.push("cancel-old");
  });
  await Promise.resolve();
  const reopened = queue("a", async () => {
    events.push("start-new");
  });
  await Promise.resolve();
  expect(events).toEqual(["start-old"]);
  release();
  await Promise.all([old, reopened]);
  expect(events).toEqual(["start-old", "cancel-old", "start-new"]);
});
