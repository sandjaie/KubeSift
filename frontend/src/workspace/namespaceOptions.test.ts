/*
Purpose: Protect explicit namespace selection with incomplete discovery.
Used by: Vitest.
Responsibilities: Verify filtering and the permission to offer unlisted namespaces.
Important: Typing produces options only; it never performs a resource read.
*/
import { expect, it } from "vitest";
import { namespaceOptions } from "./namespaceOptions";

it("filters discovered names and retains an explicit current namespace", () => {
  expect(namespaceOptions("KUBE", ["default", "kube-system"], true, "team-a")).toEqual([{ value: "kube-system", manual: false }]);
  expect(namespaceOptions("team", ["default"], true, "team-a")).toEqual([{ value: "team-a", manual: false }]);
});

it("offers valid unlisted names only when discovery is incomplete", () => {
  expect(namespaceOptions("team-b", ["default"], false, "default")).toEqual([{ value: "team-b", manual: true }]);
  expect(namespaceOptions("team-b", ["default"], true, "default")).toEqual([]);
  for (const query of ["bad/name", "UPPER", "-team", "a".repeat(64)]) {
    expect(namespaceOptions(query, [], false, "default")).toEqual([]);
  }
  expect(namespaceOptions("", ["default"], false, "default")).toEqual([{ value: "default", manual: false }]);
});
