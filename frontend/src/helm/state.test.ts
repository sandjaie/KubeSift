/*
Purpose: Verify Helm section identity across scope and refresh changes.
Used by: The Helm workspace state tests.
Responsibilities: Reject delayed replies and distinguish exact revisions.
Important: The fixture does not connect to Kubernetes.
*/
import { expect, test } from "vitest";
import { readKey, matchesRecord } from "./state";

test("another tab, driver, generation or refresh cannot reuse the old section", () => {
  const record = {
    connection: { id: "a", generation: 1 },
    namespace: "default",
    driver: "secret",
    releaseName: "demo",
    revision: "9007199254740993",
    recordName: "record",
    uid: "old",
    resourceVersion: "42",
  };
  const original = readKey("a", record, "Values", false, 0, 0);
  expect(readKey("b", record, "Values", false, 0, 0)).not.toBe(original);
  expect(readKey("a", { ...record, driver: "configmap" }, "Values", false, 0, 0)).not.toBe(original);
  expect(readKey("a", record, "Values", false, 0, 1)).not.toBe(original);
  expect(matchesRecord(record, { ...record, connection: { id: "a", generation: 2 } })).toBe(false);
  expect(matchesRecord(record, { ...record, revision: "9007199254740992" })).toBe(false);
  expect(matchesRecord(record, { ...record })).toBe(true);
});
