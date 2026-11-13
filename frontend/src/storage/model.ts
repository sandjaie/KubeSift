/*
Purpose: Describe the supported storage kinds and their captured evidence.
Used by: Storage inventory, detail and Pod Overview.
Responsibilities: Project explicit references and container volume associations.
Important: Only an explicit PVC claimName creates a Pod storage link.
*/
import type { ResourceDetail, ResourceRef } from "../workloads/model";
export type { StoragePage, StorageRow, ResolvedStorageReference } from "../../bindings/github.com/sandjaie/KubeSift/internal/kube/models";

export const storageLabels = {
  PersistentVolumeClaim: "Volume claims",
  PersistentVolume: "Persistent volumes",
  StorageClass: "Storage classes",
  VolumeAttachment: "Volume attachments",
  CSIDriver: "CSI drivers",
  CSINode: "CSI nodes",
} as const;
export type StorageKind = keyof typeof storageLabels;
export const storageKinds = Object.keys(storageLabels) as StorageKind[];
export const isStorageKind = (kind: string): kind is StorageKind => storageKinds.some((item) => item === kind);

export function storageTarget(source: ResourceRef, kind: StorageKind, name: string, namespace = "", uid = ""): ResourceRef {
  return {
    connection: source.connection,
    kind,
    name,
    namespace,
    uid,
    apiVersion: kind === "PersistentVolumeClaim" || kind === "PersistentVolume" ? "v1" : "storage.k8s.io/v1",
  };
}

export const storageClass = (name?: string | null) => (name == null ? "Not specified" : name || "No class");

type Volume = { name: string; persistentVolumeClaim?: { claimName?: string; readOnly?: boolean } };
type Container = {
  name: string;
  volumeMounts?: { name: string; mountPath: string }[];
  volumeDevices?: { name: string; devicePath: string }[];
};

export function podVolumes(data: ResourceDetail) {
  const { spec = {} } = JSON.parse(data.objectJSON) as {
    spec?: { volumes?: Volume[]; containers?: Container[]; initContainers?: Container[]; ephemeralContainers?: Container[] };
  };
  const uses = new Map<string, string[]>();
  const add = (name: string, label: string) => {
    const entries = uses.get(name);
    if (entries) entries.push(label);
    else uses.set(name, [label]);
  };
  for (const [label, containers] of [
    ["Container", spec.containers],
    ["Init container", spec.initContainers],
    ["Ephemeral container", spec.ephemeralContainers],
  ] as const) {
    for (const container of containers ?? []) {
      for (const { name, mountPath } of container.volumeMounts ?? []) add(name, `${label} ${container.name} · Mount ${mountPath}`);
      for (const { name, devicePath } of container.volumeDevices ?? []) add(name, `${label} ${container.name} · Device ${devicePath}`);
    }
  }
  return (spec.volumes ?? []).map((volume) => {
    const claim = volume.persistentVolumeClaim;
    return {
      name: volume.name,
      type:
        Object.keys(volume)
          .filter((key) => key !== "name")
          .join(", ") || "Not reported",
      target: claim?.claimName
        ? storageTarget(data.resource, "PersistentVolumeClaim", claim.claimName, data.resource.namespace)
        : undefined,
      readOnly: claim?.readOnly,
      uses: uses.get(volume.name) ?? [],
    };
  });
}
