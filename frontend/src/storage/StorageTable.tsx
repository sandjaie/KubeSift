/*
Purpose: Show storage summaries with existing explorer selection and keyboard controls.
Used by: Workspace inventory.
Responsibilities: Render only the bounded fields projected by the reader.
Important: Status and capacity columns show reported evidence.
*/
import { InventoryTable } from "../workspace/ResourceTable";
import { storageClass, storageLabels, type StorageKind, type StorageRow } from "./model";

type Summary = {
  phase?: string;
  requested?: string;
  capacity?: string;
  class?: string;
  volume?: string;
  accessModes?: string[];
  reclaimPolicy?: string;
  claim?: { name?: string; namespace?: string };
  provisioner?: string;
  defaultClass?: string;
  legacyDefault?: string;
  bindingMode?: string;
  expansion?: boolean;
  attacher?: string;
  node?: string;
  inline?: boolean;
  attached?: boolean;
  attachRequired?: boolean;
  volumeLifecycleModes?: string[];
  drivers?: number;
};
const annotations = (o: Summary) =>
  [o.defaultClass == null ? "" : `Stable: ${o.defaultClass}`, o.legacyDefault == null ? "" : `Legacy: ${o.legacyDefault}`]
    .filter(Boolean)
    .join(" · ");
const columns: Record<StorageKind, [string, (o: Summary) => string | number | boolean | undefined][]> = {
  PersistentVolumeClaim: [
    ["Phase", (o) => o.phase],
    ["Requested storage", (o) => o.requested],
    ["Reported capacity", (o) => o.capacity],
    ["Class", (o) => storageClass(o.class)],
    ["Volume", (o) => o.volume],
  ],
  PersistentVolume: [
    ["Phase", (o) => o.phase],
    ["Capacity", (o) => o.capacity],
    ["Access modes", (o) => o.accessModes?.join(", ")],
    ["Reclaim policy", (o) => o.reclaimPolicy],
    ["Class", (o) => storageClass(o.class ?? "")],
    ["Claim", (o) => (o.claim?.name ? [o.claim.namespace, o.claim.name].filter(Boolean).join("/") : undefined)],
  ],
  StorageClass: [
    ["Provisioner", (o) => o.provisioner],
    ["Default annotation", annotations],
    ["Binding mode", (o) => o.bindingMode],
    ["Reclaim policy", (o) => o.reclaimPolicy],
    ["Expansion", (o) => o.expansion],
  ],
  VolumeAttachment: [
    ["Attacher", (o) => o.attacher],
    ["Volume source", (o) => (o.inline ? "Inline volume spec" : o.volume)],
    ["Node", (o) => o.node],
    ["Attached", (o) => o.attached],
  ],
  CSIDriver: [
    ["Attach required", (o) => o.attachRequired],
    ["Lifecycle modes", (o) => o.volumeLifecycleModes?.join(", ")],
  ],
  CSINode: [["Registered drivers", (o) => o.drivers]],
};

export function StorageTable({
  rows,
  kind,
  selected,
  onSelect,
}: {
  rows: StorageRow[];
  kind: StorageKind;
  selected: string | null;
  onSelect: (row: StorageRow) => void;
}) {
  const projected = rows.map((row) => ({ ...row, summary: JSON.parse(row.objectJSON) as Summary }));
  return (
    <InventoryTable<(typeof projected)[number]>
      rows={projected}
      kind={kind}
      label={storageLabels[kind]}
      selected={selected}
      onSelect={onSelect}
      columns={columns[kind].map(([label, get]) => ({
        label,
        value: ({ summary }) => {
          const value = get(summary);
          const text = value == null || value === "" ? "Not reported" : String(value);
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
