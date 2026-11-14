/*
Purpose: Present infrastructure inventory summary columns.
Used by: Cluster and Extensions destinations.
Responsibilities: Display reported scope, quantities and condition state.
Important: Missing condition and false values remain distinct.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import { evidenceTime } from "../workspace/ResourceOverview";
import { clusterLabels, type ClusterKind, type ClusterResourceRow } from "./model";
type Summary = Record<string, string | boolean | number | null> & { service?: { name?: string; namespace?: string } };
const fields: Record<Exclude<ClusterKind, "Event">, [string, (row: Summary) => string | null | undefined][]> = {
  Node: [
    ["Ready", (v) => String(v.ready ?? "Unknown")],
    ["Scheduling disabled", (v) => (v.unschedulable == null ? "Unknown" : v.unschedulable ? "Yes" : "No")],
    ["Version", (v) => String(v.kubeletVersion ?? "Unknown")],
  ],
  PriorityClass: [
    ["Value", (v) => String(v.value ?? "Unknown")],
    ["Global default", (v) => String(v.globalDefault ?? "Not reported")],
    ["Preemption policy", (v) => String(v.preemptionPolicy ?? "Not reported")],
  ],
  RuntimeClass: [["Handler", (v) => String(v.handler ?? "Not reported")]],
  Lease: [
    ["Holder", (v) => String(v.holderIdentity ?? "Not reported")],
    ["Renew time", (v) => evidenceTime(v.renewTime)],
    ["Duration (s)", (v) => String(v.leaseDurationSeconds ?? "Not reported")],
  ],
  CustomResourceDefinition: [
    ["Group / Kind", (v) => `${v.group ?? "?"} / ${v.kind ?? "?"}`],
    ["Scope", (v) => String(v.scope ?? "Unknown")],
    ["Established", (v) => String(v.established ?? "Unknown")],
  ],
  APIService: [
    ["Group / Version", (v) => `${v.group || "core"} / ${v.version ?? "?"}`],
    ["Registration", (v) => (v.service?.name ? `${v.service.namespace ?? "?"} / ${v.service.name}` : "Local")],
    ["Available", (v) => String(v.available ?? "Unknown")],
  ],
};
export function ClusterTable({
  kind,
  rows,
  selected,
  onSelect,
}: {
  kind: Exclude<ClusterKind, "Event">;
  rows: ClusterResourceRow[];
  selected: string | null;
  onSelect: (row: ClusterResourceRow) => void;
}) {
  const projected = rows.map((row) => ({ ...row, summary: JSON.parse(row.objectJSON) as Summary }));
  return (
    <InventoryTable<(typeof projected)[number]>
      kind={kind}
      label={clusterLabels[kind]}
      rows={projected}
      selected={selected}
      onSelect={onSelect}
      columns={fields[kind].map(([label, render]) => ({ label, value: (row) => render(row.summary) }))}
    />
  );
}
