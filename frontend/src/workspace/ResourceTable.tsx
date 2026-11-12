/*
Purpose: Render one live workload page with keyboard selection.
Used by: The existing workspace list pane.
Responsibilities: Show truthful optional counts, phase and container failure reasons.
Important: Sorting and filtering apply only to this page.
*/
import type { WorkloadRow, Kind } from "../workloads/model";
import { PodPhase } from "./PodPhase";
import { ResourceIcon } from "../ui/ResourceIcon";
export function age(value: string | null) {
  if (!value) return "Unknown";
  const minutes = Math.max(0, Math.floor((Date.now() - Date.parse(value)) / 60000));
  return minutes < 60 ? `${minutes}m` : minutes < 1440 ? `${Math.floor(minutes / 60)}h` : `${Math.floor(minutes / 1440)}d`;
}
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
    <div className="resource-list">
      <table aria-label={`${kind}s`}>
        <thead>
          <tr>
            <th>Name</th>
            {kind === "Pod" ? <th>Phase / reason</th> : null}
            <th>Ready / desired</th>
            {kind === "Pod" ? (
              <>
                <th>Restarts</th>
                <th>Node</th>
              </>
            ) : (
              <>
                <th>Updated</th>
                <th>Available</th>
              </>
            )}
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, index) => (
            <tr
              key={row.resource.uid}
              aria-selected={selected === row.resource.uid}
              tabIndex={selected === row.resource.uid || (!selected && index === 0) ? 0 : -1}
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
              {kind === "Pod" ? (
                <td>
                  <PodPhase phase={row.phase} />
                  {row.reason && <span className="warning"> · {row.reason}</span>}
                </td>
              ) : null}
              <td>
                {row.ready ?? "?"} / {row.desired ?? "?"}
              </td>
              {kind === "Pod" ? (
                <>
                  <td>{row.restarts ?? "Unknown"}</td>
                  <td>{row.node || "Unknown"}</td>
                </>
              ) : (
                <>
                  <td>{row.updated ?? "Unknown"}</td>
                  <td>{row.available ?? "Unknown"}</td>
                </>
              )}
              <td>{age(row.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
