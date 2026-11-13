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
import { configMapKeys } from "./ConfigMapData";

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
