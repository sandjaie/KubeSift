/*
Purpose: Show compact networking projections using the shared inventory table.
Used by: The cluster workspace.
Responsibilities: Present only configured or captured inventory evidence.
Important: The table does not infer controller status or traffic health.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import { addressText, managerLabel, networkLabels, refText, selectorText, serviceLabel } from "./model";
import type { InventoryProjection, NetworkKind, NetworkRow } from "./model";

const headings: Record<NetworkKind, string[]> = {
  Service: ["Type", "Cluster IPs", "External address / name", "Ports"],
  EndpointSlice: ["Associated Service label", "Address type", "Endpoints", "Managed by"],
  Ingress: ["Class reference", "Hosts", "Reported addresses"],
  IngressClass: ["Controller", "Default class annotation", "Parameters"],
  NetworkPolicy: ["Pod selector", "Policy types", "Ingress rules", "Egress rules"],
};

function values(row: NetworkRow, kind: NetworkKind) {
  const raw: unknown = JSON.parse(row.objectJSON);
  switch (kind) {
    case "Service": {
      const o = raw as InventoryProjection["Service"];
      return [
        o.spec?.type,
        o.spec?.clusterIPs?.join(", ") || o.spec?.clusterIP,
        [
          o.spec?.externalName && `DNS: ${o.spec.externalName}`,
          o.spec?.externalIPs?.length && `Declared: ${o.spec.externalIPs.join(", ")}`,
          addressText(o.status?.loadBalancer?.ingress) && `Reported: ${addressText(o.status?.loadBalancer?.ingress)}`,
        ]
          .filter(Boolean)
          .join(" · ") || (o.spec?.type === "LoadBalancer" ? "None reported" : "Not specified"),
        o.spec?.ports?.map((p) => `${p.name ? p.name + ": " : ""}${p.port ?? "?"}/${p.protocol ?? "?"}`).join(", ") || "None",
      ];
    }
    case "EndpointSlice": {
      const o = raw as InventoryProjection["EndpointSlice"];
      return [o.metadata?.labels?.[serviceLabel], o.addressType, o.endpointCount, o.metadata?.labels?.[managerLabel]];
    }
    case "Ingress": {
      const o = raw as InventoryProjection["Ingress"];
      return [
        o.spec?.ingressClassName ?? "Not specified",
        o.spec?.hosts?.map((host) => host || "Any host").join(", ") || "None",
        addressText(o.status?.loadBalancer?.ingress) || "None reported",
      ];
    }
    case "IngressClass": {
      const o = raw as InventoryProjection["IngressClass"];
      return [o.spec?.controller, o.defaultClass ?? "Not specified", refText(o.spec?.parameters)];
    }
    case "NetworkPolicy": {
      const o = raw as InventoryProjection["NetworkPolicy"];
      return [
        selectorText(o.spec?.podSelector, "All Pods in this namespace"),
        o.spec?.policyTypes?.join(", ") || `API-defaulted: Ingress${o.egressRuleCount ? ", Egress" : ""}`,
        o.ingressRuleCount,
        o.egressRuleCount,
      ];
    }
  }
}

export function NetworkTable({
  rows,
  kind,
  selected,
  onSelect,
}: {
  rows: NetworkRow[];
  kind: NetworkKind;
  selected: string | null;
  onSelect: (row: NetworkRow) => void;
}) {
  const projected = rows.map((row) => ({ ...row, values: values(row, kind) }));
  return (
    <InventoryTable<(typeof projected)[number]>
      rows={projected}
      kind={kind}
      label={networkLabels[kind]}
      selected={selected}
      onSelect={onSelect}
      columns={headings[kind].map((label, i) => ({
        label,
        value: ({ values }) => {
          const value = values[i] ?? "Unknown";
          return (
            <span className="network-value" title={String(value)}>
              {value}
            </span>
          );
        },
      }))}
    />
  );
}
