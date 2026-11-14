/*
Purpose: Display storage configuration and status from one captured resource.
Used by: Resource detail and Pod storage evidence.
Responsibilities: Render explicit link targets and page nested evidence locally.
Important: Attachments, bindings and CSI registrations do not establish usable storage.
*/
import type { ReactNode } from "react";
import { EvidencePage } from "../ui/EvidencePage";
import { ResourceInspector } from "../ui/ResourceInspector";
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { evidenceTime } from "../workspace/ResourceOverview";
import { storageClass, storageTarget, type StorageKind } from "./model";

type Properties = Record<string, unknown>;
type Claim = { name?: string; namespace?: string; uid?: string; kind?: string; apiVersion?: string };
type Driver = { name?: string; nodeID?: string; topologyKeys?: string[]; allocatable?: { count?: number } };
type Capture = Properties & {
  metadata?: { annotations?: Record<string, string> };
  spec?: Properties & {
    resources?: { requests?: Properties; limits?: Properties };
    capacity?: Properties;
    claimRef?: Claim;
    storageClassName?: string;
    volumeName?: string;
    csi?: Properties & { driver?: string };
    source?: { persistentVolumeName?: string; inlineVolumeSpec?: Properties };
    attacher?: string;
    drivers?: Driver[];
  };
  status?: Properties & { capacity?: Properties };
};

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className="network-value">{children ?? "Not reported"}</dd>
    </div>
  );
}

function Value({ value }: { value: unknown }) {
  if (value == null || value === "") return "Not reported";
  if (typeof value !== "object") return String(value);
  if (Array.isArray(value) && value.length > 0 && value.length <= 10 && value.every((item) => typeof item === "string")) {
    return value.join(", ");
  }
  return <ResourceInspector value={value} empty="None reported" />;
}

const fields = (source: Properties, names: [string, string][]): [string, ReactNode][] =>
  names.map(([label, field]) => [label, <Value value={source[field]} />]);

const legacySources = [
  "gcePersistentDisk",
  "awsElasticBlockStore",
  "hostPath",
  "glusterfs",
  "nfs",
  "rbd",
  "iscsi",
  "cinder",
  "cephfs",
  "fc",
  "flocker",
  "flexVolume",
  "azureFile",
  "vsphereVolume",
  "quobyte",
  "azureDisk",
  "photonPersistentDisk",
  "portworxVolume",
  "scaleIO",
  "local",
  "storageos",
];

export function StorageOverview({ data, onOpenReference }: { data: ResourceDetail; onOpenReference: (target: ResourceRef) => void }) {
  const { resource } = data;
  const o = JSON.parse(data.objectJSON) as Capture;
  const spec = o.spec ?? {},
    status = o.status ?? {};
  const link = (kind: StorageKind, name?: string, namespace = "", uid = "") =>
    name ? (
      <button className="network-value" onClick={() => onOpenReference(storageTarget(resource, kind, name, namespace, uid))}>
        {namespace ? `${namespace}/` : ""}
        {name}
      </button>
    ) : undefined;
  const classLink = (name?: string) => link("StorageClass", name) ?? storageClass(name);
  let items: [string, ReactNode][] = [];
  let note = "";
  switch (resource.kind) {
    case "PersistentVolumeClaim":
      note = "Requested storage and reported capacity describe configuration and status. They do not measure disk usage.";
      items = [
        ["Class", classLink(spec.storageClassName)],
        ["Volume", link("PersistentVolume", spec.volumeName)],
        ["Requested storage", <Value value={spec.resources?.requests?.storage} />],
        ["Reported capacity", <Value value={status.capacity?.storage} />],
        ...fields(spec, [
          ["Requested access modes", "accessModes"],
          ["Volume mode", "volumeMode"],
          ["Selector", "selector"],
          ["Data source", "dataSource"],
          ["Data source reference", "dataSourceRef"],
          ["Requested attributes class", "volumeAttributesClassName"],
        ]),
        ...fields(status, [
          ["Phase", "phase"],
          ["Reported access modes", "accessModes"],
          ["Conditions", "conditions"],
          ["Allocated resources", "allocatedResources"],
          ["Allocation status", "allocatedResourceStatuses"],
          ["Current attributes class", "currentVolumeAttributesClassName"],
          ["Attribute change status", "modifyVolumeStatus"],
        ]),
      ];
      break;
    case "PersistentVolume": {
      note = "Capacity is provisioned space. Access modes and binding do not prove access from a Pod.";
      const claim = spec.claimRef;
      const sources = Object.fromEntries(legacySources.filter((key) => spec[key] != null).map((key) => [key, spec[key]]));
      items = [
        ["Class", classLink(spec.storageClassName ?? "")],
        [
          "Claim",
          claim?.namespace && (!claim.kind || claim.kind === "PersistentVolumeClaim") && (!claim.apiVersion || claim.apiVersion === "v1")
            ? link("PersistentVolumeClaim", claim.name, claim.namespace, claim.uid)
            : undefined,
        ],
        ["Claim reference", <Value value={claim} />],
        ["Capacity", <Value value={spec.capacity?.storage} />],
        ...fields(status, [
          ["Phase", "phase"],
          ["Reason", "reason"],
          ["Message", "message"],
          ["Conditions", "conditions"],
          ["Last phase transition", "lastPhaseTransitionTime"],
        ]),
        ...fields(spec, [
          ["Access modes", "accessModes"],
          ["Volume mode", "volumeMode"],
          ["Reclaim policy", "persistentVolumeReclaimPolicy"],
          ["Node affinity", "nodeAffinity"],
          ["Mount options", "mountOptions"],
        ]),
        ["Source kind", [spec.csi ? "csi" : "", ...Object.keys(sources)].filter(Boolean).join(", ") || undefined],
      ];
      if (spec.csi)
        items.push(
          ["CSI driver", link("CSIDriver", spec.csi.driver)],
          ...fields(spec.csi, [
            ["Volume handle", "volumeHandle"],
            ["Filesystem type", "fsType"],
            ["Read only", "readOnly"],
            ["Source attributes", "volumeAttributes"],
          ]),
        );
      if (Object.keys(sources).length) items.push(["Source configuration", <Value value={sources} />]);
      break;
    }
    case "StorageClass": {
      note = "Expansion is class configuration. WaitForFirstConsumer describes binding policy.";
      const annotations = o.metadata?.annotations;
      items = [
        ...fields(o, [
          ["Provisioner", "provisioner"],
          ["Binding mode", "volumeBindingMode"],
          ["Reclaim policy", "reclaimPolicy"],
          ["Allow expansion", "allowVolumeExpansion"],
          ["Mount options", "mountOptions"],
          ["Parameters", "parameters"],
          ["Allowed topologies", "allowedTopologies"],
        ]),
        ["Stable default annotation", annotations?.["storageclass.kubernetes.io/is-default-class"]],
        ["Legacy default annotation", annotations?.["storageclass.beta.kubernetes.io/is-default-class"]],
      ];
      break;
    }
    case "VolumeAttachment":
      note = "Reported attachment does not establish a filesystem mount or application I/O.";
      items = [
        ["Attacher", link("CSIDriver", spec.attacher)],
        ["Volume", spec.source?.inlineVolumeSpec ? "Inline volume spec" : link("PersistentVolume", spec.source?.persistentVolumeName)],
        ["Inline volume spec", <Value value={spec.source?.inlineVolumeSpec} />],
        ...fields(spec, [["Node", "nodeName"]]),
        ...fields(status, [
          ["Attached", "attached"],
          ["Attach error", "attachError"],
          ["Detach error", "detachError"],
          ["Attachment metadata", "attachmentMetadata"],
        ]),
      ];
      break;
    case "CSIDriver":
      note = "Driver configuration does not establish health or reachability.";
      items = fields(spec, [
        ["Attach required", "attachRequired"],
        ["Pod info on mount", "podInfoOnMount"],
        ["Storage capacity", "storageCapacity"],
        ["Filesystem group policy", "fsGroupPolicy"],
        ["Lifecycle modes", "volumeLifecycleModes"],
        ["Requires republish", "requiresRepublish"],
        ["SELinux mount", "seLinuxMount"],
        ["Token requests", "tokenRequests"],
        ["Node allocatable update period (seconds)", "nodeAllocatableUpdatePeriodSeconds"],
      ]);
      break;
    case "CSINode":
      note = "Registered drivers are configuration evidence. Allocatable count is a published attachment limit.";
      break;
  }
  const captureKey = JSON.stringify([resource.uid, data.resourceVersion]);
  return (
    <div className="evidence-section workload-overview" key={captureKey}>
      <p className="muted">{note}</p>
      <dl className="pod-facts">
        {items.map(([label, value]) => (
          <Fact key={label} label={label}>
            {value}
          </Fact>
        ))}
      </dl>
      {resource.kind === "CSINode" &&
        (spec.drivers?.length ? (
          <EvidencePage
            items={spec.drivers}
            render={(driver, index) => (
              <section key={index}>
                <h3>{link("CSIDriver", driver.name) ?? "Driver name not reported"}</h3>
                <dl className="pod-facts">
                  <Fact label="Node ID">
                    <Value value={driver.nodeID} />
                  </Fact>
                  <Fact label="Topology keys">
                    <Value value={driver.topologyKeys} />
                  </Fact>
                  <Fact label="Allocatable count">{driver.allocatable?.count}</Fact>
                </dl>
              </section>
            )}
          />
        ) : (
          <p>No registered drivers reported.</p>
        ))}
      <p className="muted">
        Captured {evidenceTime(data.fetchedAt)} · resourceVersion {data.resourceVersion || "Unknown"}
      </p>
    </div>
  );
}
