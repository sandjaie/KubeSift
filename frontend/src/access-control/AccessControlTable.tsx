/*
Purpose: Present the bounded access-control inventory projections.
Used by: Workspace resource table.
Responsibilities: Use shared selection and display configured counts and references.
Important: Counts are captured evidence, not a measure of effective permissions.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import { automount, accessControlLabels, type AccessControlKind, type AccessControlRow } from "./model";

type Summary = {
  automount?: boolean;
  imagePullCount?: number;
  ruleCount?: number;
  aggregation?: boolean;
  roleKind?: string;
  roleName?: string;
  subjectCount?: number;
};
const columns: Record<AccessControlKind, [string, (o: Summary) => string | number | undefined][]> = {
  ServiceAccount: [
    ["Automount configuration", (o) => automount(o.automount)],
    ["Image pull references", (o) => o.imagePullCount],
  ],
  Role: [["Rule count", (o) => o.ruleCount]],
  ClusterRole: [
    ["Rule count", (o) => o.ruleCount],
    ["Aggregation configured", (o) => (o.aggregation == null ? undefined : String(o.aggregation))],
  ],
  RoleBinding: [
    ["Role kind / name", (o) => [o.roleKind, o.roleName].filter(Boolean).join(" / ")],
    ["Subject count", (o) => o.subjectCount],
  ],
  ClusterRoleBinding: [
    ["Role name", (o) => o.roleName],
    ["Subject count", (o) => o.subjectCount],
  ],
};
export function AccessControlTable({
  rows,
  kind,
  selected,
  onSelect,
}: {
  rows: AccessControlRow[];
  kind: AccessControlKind;
  selected: string | null;
  onSelect: (row: AccessControlRow) => void;
}) {
  const projected = rows.map((row) => ({ ...row, summary: JSON.parse(row.objectJSON) as Summary }));
  return (
    <InventoryTable<(typeof projected)[number]>
      rows={projected}
      kind={kind}
      label={accessControlLabels[kind]}
      selected={selected}
      onSelect={onSelect}
      columns={columns[kind].map(([label, value]) => ({
        label,
        value: ({ summary }) => {
          const result = value(summary);
          const text = result == null || result === "" ? "Not reported" : String(result);
          return (
            <span className="network-value" title={text}>
              {text}
            </span>
          );
        },
      }))}
    />
  );
}
