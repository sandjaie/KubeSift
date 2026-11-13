/*
Purpose: Verify literal YAML search offsets against source text.
Used by: Vitest.
Responsibilities: Cover Unicode, punctuation and bounded match collection.
Important: Test strings are synthetic.
*/
import { expect, test } from "vitest";
import { yamlMatches, yamlMatchLimit } from "./yamlSearch";
test("literal matches retain source offsets and bound large results", () => {
  expect(yamlMatches("İ\nName: a.b\nname: axb\nNAME: A.B", "a.b")).toEqual([8, 28]);
  expect(yamlMatches("a", "")).toEqual([]);
  expect(yamlMatches("a", "b")).toEqual([]);
  expect(yamlMatches("a".repeat(20000), "a")).toHaveLength(yamlMatchLimit + 1);
});
