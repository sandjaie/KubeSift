/*
Purpose: Show bounded configuration inventory with shared keyboard selection.
Used by: The resource workspace.
Responsibilities: Render projected columns and a metadata-only Secret context.
Important: Inventory has no captured values.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import { configurationLabels, scopeSummary, type ConfigurationKind, type ConfigurationRow, type ScopeSelector } from "./model";

const columns: Record<ConfigurationKind, string[]> = {
  Namespace: ["Phase"],
  ResourceQuota: ["Resources", "Scope"],
  LimitRange: ["Item types"],
  ConfigMap: ["Text keys", "Binary keys", "Immutable"],
  Secret: [],
};
type Projection = {
  phase?: string;
  resources?: number;
  scopes?: string[];
  scopeSelector?: ScopeSelector;
  types?: string[];
  textKeys?: number;
  binaryKeys?: number;
  immutable?: boolean;
};
function values(kind: ConfigurationKind, o: Projection) {
  switch (kind) {
    case "Namespace":
      return [o.phase || "Unknown"];
    case "ResourceQuota":
      return [o.resources, scopeSummary(o.scopes, o.scopeSelector)];
    case "LimitRange":
      return [o.types?.join(", ") || "None specified"];
    case "ConfigMap":
      return [o.textKeys, o.binaryKeys, o.immutable == null ? "Not reported" : String(o.immutable)];
    case "Secret":
      return [];
  }
}
export function ConfigurationTable({
  rows,
  kind,
  selected,
  onSelect,
}: {
  rows: ConfigurationRow[];
  kind: ConfigurationKind;
  selected: string | null;
  onSelect: (row: ConfigurationRow) => void;
}) {
  const projected = rows.map((row) => ({ ...row, values: values(kind, JSON.parse(row.objectJSON) as Projection) }));
  return (
    <InventoryTable<(typeof projected)[number]>
      rows={projected}
      kind={kind}
      label={configurationLabels[kind]}
      selected={selected}
      onSelect={onSelect}
      columns={columns[kind].map((label, index) => ({
        label,
        value: ({ values }) => (
          <span className="network-value" title={String(values[index] ?? "Unknown")}>
            {values[index] ?? "Unknown"}
          </span>
        ),
      }))}
    />
  );
}
