/*
Purpose: Define and render the resource navigation registry.
Used by: Workspace.
Responsibilities: Group resource destinations and identify implemented live destinations.
Important: Unimplemented kinds never display invented live data.
*/
import { useId } from "react";
import { ResourceIcon } from "../ui/ResourceIcon";
import { storageKinds, storageLabels } from "../storage/model";
import { accessControlKinds, accessControlLabels } from "../access-control/model";
import { availabilityKinds, availabilityLabels } from "../availability/model";
import { clusterLabels } from "../cluster/model";
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
      ...(["Node", "Event", "PriorityClass", "RuntimeClass", "Lease"] as const).map((kind) => [kind, clusterLabels[kind], true] as const),
    ],
  },
  {
    group: "Extensions",
    items: [
      ["CustomResourceDefinition", clusterLabels.CustomResourceDefinition, true],
      ["APIService", clusterLabels.APIService, true],
      ["APIResources", "API resources", true],
      ["Helm", "Helm", true],
    ],
  },
] as const;
export function ResourceNavigation({
  kind,
  onSelect,
  projects = false,
  collapsedGroups,
  onToggleGroup,
}: {
  kind: string;
  onSelect: (kind: string, available: boolean) => void;
  projects?: boolean;
  collapsedGroups: readonly string[];
  onToggleGroup: (group: string) => void;
}) {
  const id = useId();
  return (
    <nav className="resource-nav" aria-label="Resources">
      {[...destinations, ...(projects ? [{ group: "OpenShift", items: [["Project", "Projects", true] as const] }] : [])].map(
        ({ group, items }, index) => {
          const expanded = !collapsedGroups.includes(group);
          const listID = `${id}-${index}`;
          return (
            <section key={group}>
              <h3>
                <button
                  type="button"
                  aria-expanded={expanded}
                  aria-controls={listID}
                  data-current={!expanded && items.some(([key]) => kind === key) ? true : undefined}
                  onClick={() => onToggleGroup(group)}
                >
                  <span aria-hidden="true" className="nav-chevron" />
                  {group}
                </button>
              </h3>
              <div id={listID} hidden={!expanded}>
                {items.map(([key, label, available]) => (
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
              </div>
            </section>
          );
        },
      )}
    </nav>
  );
}
