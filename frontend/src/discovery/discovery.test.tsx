/*
Purpose: Verify descriptor matching and namespace isolation in installed API views.
Used by: Frontend regression checks.
Responsibilities: Reject stale scope and show exact, inert evidence.
Important: This file uses synthetic captures only.
*/
import { expect, test } from "vitest";
import { selectCatalogIntent, visibleGenericRoute } from "./model";
const descriptor = {
  group: "example.test",
  version: "v1",
  resource: "widgets",
  kind: "Pod",
  namespaced: true,
  handle: "h",
  mode: "generic",
  reason: "",
  list: true,
  get: true,
};
const catalog = { resources: [descriptor], epoch: "e", fetchedAt: "" };
test("CRD intents require a canonical group, plural, kind and scope match", () => {
  expect(
    selectCatalogIntent({ group: "example.test", version: "v1", match: { resource: "widgets", kind: "Pod", namespaced: true } }, catalog)
      .descriptor,
  ).toEqual(descriptor);
  expect(() =>
    selectCatalogIntent({ group: "example.test", version: "v1", match: { resource: "widgets", kind: "Other", namespaced: true } }, catalog),
  ).toThrow();
});
test("namespace change keeps the descriptor and clears namespaced page history only", () => {
  const route = { group: "example.test", version: "v1", descriptor, namespace: "first", history: ["", "next"] };
  expect(visibleGenericRoute(route, "second")).toMatchObject({ descriptor, namespace: "second", history: [""] });
  expect(visibleGenericRoute({ ...route, descriptor: { ...descriptor, namespaced: false } }, "second").history).toEqual(["", "next"]);
});
