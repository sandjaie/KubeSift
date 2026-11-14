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

test("departed requests cannot deliver late success or failure even if cancellation races", async () => {
  const { newestRead } = await import("./model");
  const gate = newestRead();
  let finishOld!: (value: string) => void;
  let failOld!: (error: Error) => void;
  let finishNew!: (value: string) => void;
  let cancellations = 0;
  const results: string[] = [];
  const onError = (error: unknown) => results.push(String(error));
  gate.start(
    {
      promise: new Promise<string>((resolve) => {
        finishOld = resolve;
      }),
      cancel: () => {
        cancellations++;
      },
    },
    (value) => results.push(value),
    onError,
  );
  gate.start(
    {
      promise: new Promise<string>((_resolve, reject) => {
        failOld = reject;
      }),
      cancel: () => {
        cancellations++;
      },
    },
    (value) => results.push(value),
    onError,
  );
  gate.start(
    {
      promise: new Promise<string>((resolve) => {
        finishNew = resolve;
      }),
      cancel: () => {
        cancellations++;
      },
    },
    (value) => results.push(value),
    onError,
  );
  finishNew("new connection");
  finishOld("old connection");
  failOld(new Error("late denial"));
  await Promise.resolve();
  expect(results).toEqual(["new connection"]);
  expect(cancellations).toBe(2);
});

test("expired and replaced descriptors do not restore a generic capture", async () => {
  const { currentGenericDescriptor } = await import("./model");
  expect(currentGenericDescriptor(descriptor, catalog)).toEqual(descriptor);
  expect(currentGenericDescriptor(descriptor, { ...catalog, resources: [{ ...descriptor, handle: "replacement" }] })).toBeUndefined();
  expect(currentGenericDescriptor(descriptor, { ...catalog, resources: [] })).toBeUndefined();
});
