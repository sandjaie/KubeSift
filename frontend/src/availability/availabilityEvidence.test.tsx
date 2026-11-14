/*
Purpose: Verify captured availability presentation.
Used by: Frontend regression checks.
Responsibilities: Distinguish stale or absent evidence and keep opaque history inert.
Important: The tests never query metrics or open non-explicit references.
*/
import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import type { ResourceDetail } from "../workloads/model";
import { AvailabilityOverview } from "./AvailabilityOverview";
import { availabilityTarget } from "./model";

const capture = (kind: string, value: unknown): ResourceDetail => ({
  resource: { connection: { id: "test", generation: 1 }, name: "sample", namespace: "test", uid: "uid", apiVersion: "v1", kind },
  fetchedAt: "",
  resourceVersion: "1",
  job: null,
  objectJSON: JSON.stringify(value),
  availability: { observation: "stale", revision: "9007199254740993" },
});
const render = (data: ResourceDetail) => renderToStaticMarkup(<AvailabilityOverview data={data} onOpenReference={() => {}} />);

test("PDB selector tri-state and reported zero do not infer eviction eligibility", () => {
  expect(render(capture("PodDisruptionBudget", { spec: { selector: null } }))).toContain("Selects no Pods");
  const view = render(
    capture("PodDisruptionBudget", {
      spec: { selector: {}, maxUnavailable: "10%", unhealthyPodEvictionPolicy: "FuturePolicy" },
      status: { disruptionsAllowed: 0 },
    }),
  );
  for (const token of ["Selects all Pods in this namespace", "10%", "FuturePolicy", ">0<", "Observation differs"])
    expect(view).toContain(token);
});

test("revision is exact and opaque, with unsupported and ambiguous owners kept inert", () => {
  const owner = { kind: "StatefulSet", apiVersion: "apps/v1", uid: "owner", name: "set", controller: true };
  const data = capture("ControllerRevision", {
    revision: 9007199254740992,
    data: { nested: ["do-not-mount"] },
    metadata: { ownerReferences: [owner, owner] },
  });
  const view = render(data);
  expect(view).toContain("9007199254740993");
  expect(view).not.toContain("do-not-mount");
  expect(view).not.toContain("<button");
  expect(availabilityTarget(data.resource, owner)?.uid).toBe("owner");
});

test("HPA metric targets and observations keep independent sources and tolerate null arrays", () => {
  const types = ["Resource", "ContainerResource", "Pods", "Object", "External", "FutureSource"].map((type) => ({ type }));
  const data = capture("HorizontalPodAutoscaler", {
    spec: { metrics: types, scaleTargetRef: { kind: "Custom", apiVersion: "other/v1", name: "inert" } },
    status: { currentMetrics: null, currentReplicas: 0 },
  });
  const view = render(data);
  for (const name of ["Metric targets", "Reported metrics", "ContainerResource", "FutureSource", "Not reported", ">0<"])
    expect(view).toContain(name);
  expect(view).not.toContain("<button");
});

test("revision metadata safely displays a null owner entry", () => {
  const view = render(capture("ControllerRevision", { metadata: { ownerReferences: [null] } }));
  expect(view).toContain("Captured owner references");
  expect(view).not.toContain("<button");
});

test("structured generation evidence uses native decimal strings", () => {
  const data = capture("HorizontalPodAutoscaler", { status: { observedGeneration: 9007199254740992 } });
  Object.assign(data.availability!, { generation: "9007199254740994", observedGeneration: "9007199254740993" });
  const view = render(data);
  expect(view).toContain("9007199254740994");
  expect(view).toContain("9007199254740993");
  expect(view).not.toContain("9007199254740992");
});
