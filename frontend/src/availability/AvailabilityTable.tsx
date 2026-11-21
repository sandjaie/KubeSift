/*
Purpose: Render bounded availability inventory summaries.
Used by: Workspace inventory.
Responsibilities: Show reported values without opening nested evidence.
Important: Missing values stay unknown and revision text stays exact.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import { availabilityLabels, observationLabel, type AvailabilityKind, type AvailabilityRow, type Reference } from "./model";

type Summary = {
  target?: Reference;
  owner?: Reference;
  min?: string | number;
  max?: string | number;
  current?: number;
  desired?: number;
  allowed?: number;
};
const show = (value: unknown) => value ?? "Not reported";

export function AvailabilityTable({
  rows,
  kind,
  selected,
  onSelect,
}: {
  rows: AvailabilityRow[];
  kind: AvailabilityKind;
  selected: string | null;
  onSelect: (row: AvailabilityRow) => void;
}) {
  const projected = rows.map((row) => ({ ...row, captured: JSON.parse(row.objectJSON) as Summary }));
  const observation = { label: "Observation", value: (row: AvailabilityRow) => observationLabel(row.summary.observation) };
  const reference = (ref?: Reference) => (ref?.name ? `${ref.kind} / ${ref.name}` : "Not reported");
  return (
    <InventoryTable<(typeof projected)[number]>
      rows={projected}
      kind={kind}
      label={availabilityLabels[kind]}
      selected={selected}
      onSelect={onSelect}
      columns={
        kind === "HorizontalPodAutoscaler"
          ? [
              { label: "Target", value: ({ captured }) => reference(captured.target) },
              { label: "Min / Max", value: ({ captured }) => `${show(captured.min)} / ${show(captured.max)}` },
              { label: "Reported current / desired", value: ({ captured }) => `${show(captured.current)} / ${show(captured.desired)}` },
              observation,
            ]
          : kind === "PodDisruptionBudget"
            ? [
                { label: "Min available / Max unavailable", value: ({ captured }) => `${show(captured.min)} / ${show(captured.max)}` },
                { label: "Reported disruptions allowed", value: ({ captured }) => captured.allowed },
                observation,
              ]
            : [
                { label: "Revision", value: ({ summary }) => summary.revision || "Not reported" },
                { label: "Controller owner", value: ({ captured }) => reference(captured.owner) },
              ]
      }
    />
  );
}
