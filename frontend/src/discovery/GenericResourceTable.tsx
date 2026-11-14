/*
Purpose: Render metadata for a generic API page.
Used by: Installed API inventory.
Responsibilities: Reuse keyboard selection and backend-projected object addresses.
Important: Rows contain no spec or status.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import type { APIResourceDescriptor, GenericResourceRow } from "./model";
export function GenericResourceTable({
  rows,
  descriptor,
  selected,
  onSelect,
}: {
  rows: GenericResourceRow[];
  descriptor: APIResourceDescriptor;
  selected: string | null;
  onSelect: (row: GenericResourceRow) => void;
}) {
  return (
    <InventoryTable
      rows={rows}
      kind={descriptor.kind}
      label={descriptor.resource}
      selected={selected}
      onSelect={onSelect}
      columns={descriptor.namespaced ? [{ label: "Namespace", value: (row) => row.resource.namespace }] : []}
    />
  );
}
