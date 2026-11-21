/*
Purpose: Verify infrastructure overview renders reported facts.
Used by: Frontend regression checks.
Responsibilities: Check absent and zero evidence and collapsed schema.
Important: No cluster is contacted.
*/
import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { ResourceInspector } from "../ui/ResourceInspector";
import { ClusterOverview } from "./ClusterOverview";
import type { ResourceDetail } from "../workloads/model";

const render = (kind: string, body: unknown) =>
  renderToStaticMarkup(
    <ClusterOverview
      data={
        {
          resource: { kind, name: "sample", namespace: "", uid: "id", apiVersion: "v1", connection: { id: "test", generation: 1 } },
          fetchedAt: "",
          resourceVersion: "1",
          job: null,
          objectJSON: JSON.stringify(body),
        } satisfies ResourceDetail
      }
      onTarget={() => {}}
      onBrowse={() => {}}
    />,
  );
test("Node scheduling and unknown readiness are separate", () => {
  const view = render("Node", { spec: { unschedulable: false }, status: { capacity: { memory: "98Gi" } } });
  for (const value of ["Scheduling disabled", "false", "Unknown", "Capacity and allocatable", "current usage"])
    expect(view).toContain(value);
});
test("Lease shows zero duration without asserting leadership", () => {
  const view = render("Lease", { spec: { leaseDurationSeconds: 0 } });
  expect(view).toContain(">0<");
  expect(view).toContain("do not establish");
});
test("CRD schema is absent until expanded", () => {
  const view = render("CustomResourceDefinition", {
    spec: { versions: [{ name: "v1", served: true, schema: { marker: "hidden-value" } }] },
  });
  expect(view).toContain("Schema and conversion");
  expect(view).not.toContain("hidden-value");
});

test("inspector distinguishes explicit JSON scalars and missing evidence", () => {
  const value = (captured: unknown) => {
    const html = renderToStaticMarkup(<ResourceInspector label="Field" value={captured} />);
    return html.match(/class="inspector-value">(.*?)<\/span>/)?.[1];
  };
  expect([value(null), value(undefined), value("null"), value(1), value("1"), value(true), value("true")]).toEqual([
    "null",
    "Not reported",
    "&quot;null&quot;",
    "1",
    "&quot;1&quot;",
    "true",
    "&quot;true&quot;",
  ]);
});
