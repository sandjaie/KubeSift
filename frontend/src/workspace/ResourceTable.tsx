/*
Purpose: Render one live workload page with keyboard selection.
Used by: The existing workspace list pane.
Responsibilities: Show truthful optional counts, phase and container failure reasons.
Important: Sorting and filtering apply only to this page.
*/
import type { WorkloadRow, Kind } from "../workloads/model";
import type { ReactNode } from "react";
import { evidenceTime } from "./ResourceOverview";
import { PodPhase } from "./PodPhase";
import { ResourceIcon } from "../ui/ResourceIcon";
export function age(value: string | null) {
  if (!value || !Number.isFinite(Date.parse(value))) return "Unknown";
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(value)) / 60000));
  return minutes < 60 ? `${minutes}m` : minutes < 1440 ? `${Math.floor(minutes / 60)}h` : `${Math.floor(minutes / 1440)}d`;
}
type Column<T = WorkloadRow> = { label: string; value: (row: T) => ReactNode };
const ready: Column = { label: "Ready / desired", value: (row) => `${row.ready ?? "?"} / ${row.desired ?? "?"}` };
const updated: Column = { label: "Updated", value: (row) => row.updated };
const available: Column = { label: "Available", value: (row) => row.available };

export const jobColumns: Column[] = [
  { label: "Status", value: (row) => row.job?.status },
  { label: "Succeeded / completions", value: (row) => `${row.job?.succeeded ?? "Unknown"} / ${row.job?.completions ?? "Not specified"}` },
  { label: "Active Pods", value: (row) => row.job?.active },
  { label: "Failed attempts", value: (row) => row.job?.failed },
];

const workloadColumns: Record<Kind, Column[]> = {
  Pod: [
    {
      label: "Phase / reason",
      value: (row) => (
        <>
          <PodPhase phase={row.phase} />
          {row.reason && <span className="warning"> · {row.reason}</span>}
        </>
      ),
    },
    ready,
    { label: "Restarts", value: (row) => row.restarts },
    { label: "Node", value: (row) => row.node || "Unknown" },
  ],
  Deployment: [ready, updated, available],
  ReplicaSet: [
    { label: "Desired", value: (row) => row.desired },
    { label: "Current", value: (row) => row.current },
    { label: "Ready", value: (row) => row.ready },
    available,
  ],
  StatefulSet: [ready, { label: "Total Pods", value: (row) => row.total }, { ...updated, label: "Updated Pods" }],
  DaemonSet: [
    { ...ready, label: "Ready / desired nodes" },
    { label: "Scheduled nodes", value: (row) => row.current },
    { ...updated, label: "Updated nodes" },
  ],
  Job: jobColumns,
  CronJob: [
    { label: "Schedule", value: (row) => row.cronJob?.schedule },
    { label: "Time zone", value: (row) => row.cronJob?.timeZone ?? "Controller default (unknown)" },
    { label: "Suspended", value: (row) => (row.cronJob?.suspended == null ? "Unknown" : row.cronJob.suspended ? "Yes" : "No") },
    { label: "Active Job references", value: (row) => row.cronJob?.active },
    { label: "Last scheduled", value: (row) => evidenceTime(row.cronJob?.lastScheduled) },
  ],
};
export function ResourceTable({
  rows,
  kind,
  selected,
  onSelect,
}: {
  rows: WorkloadRow[];
  kind: Kind;
  selected: string | null;
  onSelect: (row: WorkloadRow) => void;
}) {
  return (
    <InventoryTable rows={rows} kind={kind} label={`${kind}s`} selected={selected} onSelect={onSelect} columns={workloadColumns[kind]} />
  );
}

export function InventoryTable<T extends Pick<WorkloadRow, "resource" | "createdAt">>({
  rows,
  kind,
  label,
  selected,
  onSelect,
  columns,
}: {
  rows: T[];
  kind: string;
  label: string;
  selected: string | null;
  onSelect: (row: T) => void;
  columns: Column<T>[];
}) {
  const selectedVisible = rows.some((row) => row.resource.uid === selected);
  return (
    <div className="resource-list">
      <table aria-label={label}>
        <thead>
          <tr>
            <th>Name</th>
            {columns.map(({ label }) => (
              <th key={label}>{label}</th>
            ))}
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr
              key={row.resource.uid}
              aria-selected={selected === row.resource.uid}
              tabIndex={selected === row.resource.uid || (!selectedVisible && index === 0) ? 0 : -1}
              onClick={() => onSelect(row)}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                  e.preventDefault();
                  const next = Math.max(0, Math.min(rows.length - 1, index + (e.key === "ArrowDown" ? 1 : -1)));
                  onSelect(rows[next]);
                  (e.currentTarget.parentElement?.children[next] as HTMLElement)?.focus();
                }
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onSelect(row);
                }
              }}
            >
              <td>
                <strong>
                  <ResourceIcon kind={kind} size={14} /> {row.resource.name}
                </strong>
              </td>
              {columns.map(({ label, value }) => (
                <td key={label}>{value(row) ?? "Unknown"}</td>
              ))}
              <td>{age(row.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
