/*
Purpose: Define and render the resource navigation registry.
Used by: Workspace.
Responsibilities: Group resource destinations and identify implemented live destinations.
Important: Unimplemented kinds never display invented live data.
*/
import { ResourceIcon } from "../ui/ResourceIcon";
import { storageKinds, storageLabels } from "../storage/model";
import { accessControlKinds, accessControlLabels } from "../access-control/model";
import { availabilityKinds, availabilityLabels } from "../availability/model";
export const destinations = [
  {
    group: "Workloads",
    items: [
      ["Pod", "Pods", true],
      ["Deployment", "Deployments", true],
      ["ReplicaSet", "ReplicaSets", true],
      ["ReplicationController", "ReplicationControllers", true],
      ["StatefulSet", "StatefulSets", true],
      ["DaemonSet", "DaemonSets", true],
      ["Job", "Jobs", true],
      ["CronJob", "CronJobs", true],
      ...availabilityKinds.map((kind) => [kind, availabilityLabels[kind], true] as const),
      ["ConfigMap", "ConfigMaps", true],
      ["Secret", "Secrets", true],
    ],
  },
  {
    group: "Networking",
    items: [
      ["Service", "Services", true],
      ["EndpointSlice", "EndpointSlices", true],
      ["Ingress", "Ingresses", true],
      ["IngressClass", "IngressClasses", true],
      ["NetworkPolicy", "NetworkPolicies", true],
    ],
  },
  {
    group: "Configuration",
    items: [
      ["ResourceQuota", "ResourceQuotas", true],
      ["LimitRange", "LimitRanges", true],
    ],
  },
  { group: "Storage", items: storageKinds.map((kind) => [kind, storageLabels[kind], true] as const) },
  { group: "Access control", items: accessControlKinds.map((kind) => [kind, accessControlLabels[kind], true] as const) },
  {
    group: "Cluster",
    items: [
      ["Namespace", "Namespaces", true],
      ["Node", "Nodes", false],
    ],
  },
  {
    group: "Extensions",
    items: [
      ["Widget", "Custom resources", false],
      ["Helm", "Helm", false],
      ["OpenShift", "OpenShift", false],
    ],
  },
] as const;
export function ResourceNavigation({ kind, onSelect }: { kind: string; onSelect: (kind: string, available: boolean) => void }) {
  return (
    <nav className="resource-nav" aria-label="Resources">
      {destinations.map((g) => (
        <section key={g.group}>
          <h3>{g.group}</h3>
          {g.items.map(([key, label, available]) => (
            <button key={key} disabled={!available} aria-pressed={kind === key} onClick={() => onSelect(key, available)}>
              <ResourceIcon kind={key} />
              {label}
              {!available && (
                <span className="future" title="Not available yet">
                  Soon
                </span>
              )}
            </button>
          ))}
        </section>
      ))}
    </nav>
  );
}
