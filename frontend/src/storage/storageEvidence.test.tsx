/*
Purpose: Verify storage views preserve optional evidence and explicit links.
Used by: Frontend storage regression checks.
Responsibilities: Exercise rendering, bounded nested rows and Pod volume projection.
Important: Synthetic evidence does not represent a working storage driver.
*/
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { ResourceDetail } from "../workloads/model";
import { podVolumes, storageClass } from "./model";
import { StorageOverview } from "./StorageOverview";

function capture(kind: string, body: unknown): ResourceDetail {
  return {
    resource: { connection: { id: "test", generation: 1 }, kind, apiVersion: "v1", namespace: "test", name: "sample", uid: "uid" },
    resourceVersion: "1",
    fetchedAt: "",
    objectJSON: JSON.stringify(body),
    job: null,
  };
}
const overview = (kind: string, body: unknown) =>
  renderToStaticMarkup(<StorageOverview data={capture(kind, body)} onOpenReference={() => {}} />);

describe("storage evidence", () => {
  it("distinguishes omitted class, capacity and status from explicit values", () => {
    expect([storageClass(undefined), storageClass(""), storageClass("fast")]).toEqual(["Not specified", "No class", "fast"]);
    const claim = overview("PersistentVolumeClaim", {
      spec: { resources: { requests: { storage: "1.50Gi" } } },
      status: { capacity: { storage: "0" } },
    });
    expect(claim).toContain("1.50Gi");
    expect(claim).toContain(">0<");
    expect(overview("VolumeAttachment", { status: {} })).not.toContain(">false<");
    expect(overview("VolumeAttachment", { status: { attached: false } })).toContain(">false<");
    expect(overview("CSIDriver", { spec: {} })).not.toContain(">false<");
  });

  it("keeps nested driver entries independent and hostile configuration inert", () => {
    const drivers = Array.from({ length: 101 }, (_, n) => ({
      name: `driver-${n}`,
      nodeID: "<script>x</script>",
      allocatable: n === 0 ? { count: 0 } : {},
    }));
    const node = overview("CSINode", { spec: { drivers } });
    expect(node).toContain("&lt;script&gt;");
    expect(node).toContain(">0<");
    expect(node).not.toContain("driver-100");
    expect(node).toContain("Next");
    expect(overview("StorageClass", { provisioner: "driver", parameters: { endpoint: "https://example.test" } })).not.toContain("<a ");
  });

  it("does not mount the descendants of collapsed storage disclosures", () => {
    const topologies = Array.from({ length: 100 }, () => ({
      matchLabelExpressions: Array.from({ length: 20 }, () => ({ key: "fixture", values: ["deep-sentinel"] })),
    }));
    const markup = overview("StorageClass", { allowedTopologies: topologies });
    expect(markup).toContain("Show 100 entries");
    expect(markup.includes("deep-sentinel")).toBe(false);
    expect(markup.length).toBeLessThan(3000);
  });

  it("recognizes typed PV sources without assigning meaning to future spec fields", () => {
    const markup = overview("PersistentVolume", { spec: { hostPath: { path: "/fixture" }, futureField: { value: "ignored" } } });
    expect(markup).toContain("hostPath");
    expect(markup).not.toContain("futureField");
  });

  it("associates mounts and devices across container types without inferring ephemeral claims", () => {
    const rows = podVolumes(
      capture("Pod", {
        spec: {
          volumes: [
            { name: "claim", persistentVolumeClaim: { claimName: "data", readOnly: true } },
            { name: "ephemeral", ephemeral: {} },
            { name: "inline", csi: {} },
          ],
          containers: [{ name: "main", volumeMounts: [{ name: "claim", mountPath: "/data" }] }],
          initContainers: [{ name: "init", volumeDevices: [{ name: "claim", devicePath: "/dev/data" }] }],
          ephemeralContainers: [{ name: "debug", volumeMounts: [{ name: "claim", mountPath: "/debug" }] }],
        },
      }),
    );
    expect(rows.map(({ type, target }) => [type, target?.name])).toEqual([
      ["persistentVolumeClaim", "data"],
      ["ephemeral", undefined],
      ["csi", undefined],
    ]);
    expect(rows[0].uses).toHaveLength(3);
    expect(rows[0].target?.namespace).toBe("test");
    expect(rows[0].readOnly).toBe(true);
  });
});
