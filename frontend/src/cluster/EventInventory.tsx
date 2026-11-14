/*
Purpose: Present one namespace Event inventory page.
Used by: Cluster Events.
Responsibilities: Show page order, observed time and the recorded regarding object.
Important: No whole-cluster or globally latest Event promise.
*/
import { InventoryTable, age } from "../workspace/ResourceTable";
import { evidenceTime } from "../workspace/ResourceOverview";
import type { EventInventoryRow } from "./model";

export function EventInventory({
  rows,
  selected,
  onSelect,
}: {
  rows: EventInventoryRow[];
  selected: string | null;
  onSelect: (row: EventInventoryRow) => void;
}) {
  return (
    <InventoryTable
      kind="Event"
      label="Events"
      selected={selected}
      onSelect={onSelect}
      rows={rows.map((row) => ({ ...row, createdAt: row.event.firstAt }))}
      columns={[
        { label: "Type", value: (row) => row.event.type },
        { label: "Reason", value: (row) => row.event.reason },
        { label: "Message", value: (row) => (row.event.message.length > 160 ? row.event.message.slice(0, 160) + "…" : row.event.message) },
        { label: "Regarding", value: (row) => row.regarding },
        { label: "Count", value: (row) => row.event.count },
        { label: "Last observed", value: (row) => <span title={evidenceTime(row.event.lastAt)}>{age(row.event.lastAt)}</span> },
      ]}
    />
  );
}
