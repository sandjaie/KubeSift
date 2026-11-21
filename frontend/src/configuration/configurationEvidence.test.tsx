/*
Purpose: Check literal configuration evidence and metadata-only Secret markup.
Used by: Frontend regression tests.
Responsibilities: Assert missing values, quota/limit boundaries and captured key handling.
Important: All values are synthetic.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import type { ResourceDetail } from "../workloads/model";
import { ConfigurationOverview } from "./ConfigurationOverview";
import { ConfigMapData, configMapKeys } from "./ConfigMapData";

function detail(kind: string, object: unknown): ResourceDetail {
  return {
    resource: { connection: { id: "a", generation: 1 }, kind, apiVersion: "v1", name: "example", namespace: "test", uid: "uid" },
    resourceVersion: "1",
    fetchedAt: "",
    job: null,
    objectJSON: JSON.stringify(object),
  };
}
function overview(kind: string, object: unknown) {
  return renderToStaticMarkup(<ConfigurationOverview data={detail(kind, object)} onBrowseNamespace={() => {}} />);
}
test("quota shows separate quantities and union keys without inventing status", () => {
  const html = overview("ResourceQuota", {
    spec: { hard: { "example/custom": "0", cpu: "500m" } },
    status: { hard: { cpu: "1" }, used: { memory: "0" } },
  });
  for (const value of ["example/custom", "500m", "memory", "Not reported", ">0<"]) expect(html).toContain(value);
});
test("limits retain item boundaries and literal defaults", () => {
  const html = overview("LimitRange", {
    spec: {
      limits: [
        { type: "Container", defaultRequest: { cpu: "100m" } },
        { type: "Pod", maxLimitRequestRatio: { memory: "4" } },
      ],
    },
  });
  for (const value of ["Container", "Pod", "100m", "Default request", "Max limit/request ratio"]) expect(html).toContain(value);
});
test("Secret overview ignores arbitrary fields even if present in the capture", () => {
  const html = overview("Secret", { metadata: { annotations: { private: "sentinel" } }, type: "Opaque", data: { password: "sentinel" } });
  expect(html).toContain("Only identity metadata");
  expect(html).not.toMatch(/sentinel|Opaque|password|Copy value/);
});
test("ConfigMap keys preserve literal content and count decoded bytes", () => {
  const keys = configMapKeys(detail("ConfigMap", { data: { text: "☃\n<script>", empty: "" }, binaryData: { bytes: "AP8=" } }));
  expect(keys.map(({ key, bytes, value }) => [key, bytes, value])).toEqual([
    ["bytes", 2, "AP8="],
    ["empty", 0, ""],
    ["text", 12, "☃\n<script>"],
  ]);
});

test("ConfigMap Data exposes inline values and direct copy while deferring long values", () => {
  const html = renderToStaticMarkup(
    <ConfigMapData
      data={detail("ConfigMap", {
        data: { literal: "<script>", multiline: "hidden-line\nnext", empty: "" },
        binaryData: { bytes: "AP8=" },
      })}
    />,
  );
  for (const expected of [
    "4 keys",
    "&lt;script&gt;",
    "AP8=",
    "Empty value",
    "View multiline",
    "Copy value for multiline",
    "Copy base64 for bytes",
  ]) {
    expect(html).toContain(expected);
  }
  expect(html).not.toContain("hidden-line");
  expect(html).not.toContain("<script>");
});

test("ConfigMap annotations start closed and keep an embedded document out of the initial markup", () => {
  const html = overview("ConfigMap", {
    metadata: { annotations: { "kubectl.kubernetes.io/last-applied-configuration": '{"data":{"key":"annotation-literal"}}' } },
  });
  expect(html).toContain("Annotations </span>");
  expect(html).not.toContain("annotation-literal");
});

test("Pod reference projection preserves duplicate locations and ignores plugin references", async () => {
  const { podConfigurationReferences } = await import("./PodConfiguration");
  const rows = podConfigurationReferences(
    detail("Pod", {
      spec: {
        containers: [
          { name: "main", env: [{ name: "VALUE", valueFrom: { secretKeyRef: { name: "shared", key: "key", optional: false } } }] },
        ],
        volumes: [
          { name: "volume", secret: { secretName: "shared", optional: true } },
          { name: "plugin", csi: { nodePublishSecretRef: { name: "unsupported" } } },
        ],
      },
    }),
  );
  expect(rows.map(({ resource, location, keys, optional }) => [resource.kind, resource.name, location, keys, optional])).toEqual([
    ["Secret", "shared", "Container main · env VALUE", "key", false],
    ["Secret", "shared", "Volume volume", undefined, true],
  ]);
});
