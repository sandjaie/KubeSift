/*
Purpose: Show a bounded batch of CronJob-owned Jobs.
Used by: The CronJob Jobs tab.
Responsibilities: Order the displayed batch and open captured Job identities.
Important: Retained Jobs are incomplete scheduling history.
*/
import type { RelatedJobPage, ResourceRef } from "../workloads/model";
import { evidenceTime } from "./ResourceOverview";
import { jobColumns } from "./ResourceTable";

export function RelatedJobs({
  data,
  onNext,
  onSelect,
}: {
  data: RelatedJobPage;
  onNext: (cursor: string) => void;
  onSelect: (resource: ResourceRef) => void;
}) {
  const time = (value: string | null) => {
    const milliseconds = Date.parse(value ?? "");
    return Number.isFinite(milliseconds) ? milliseconds : -Infinity;
  };
  const rows = [...(data.items ?? [])].sort(
    (a, b) => time(b.createdAt) - time(a.createdAt) || a.resource.uid.localeCompare(b.resource.uid),
  );
  return (
    <div className="evidence-section">
      {data.problem && <p role="alert">{data.problem.message}</p>}
      {!data.problem && !rows.length && (
        <p>{data.complete ? "No retained owned Jobs found." : "No owned Jobs in this batch. More results may be available."}</p>
      )}
      {!!rows.length && (
        <>
          <p className="muted">{rows.length} shown · Newest first in this batch</p>
          <div className="evidence-table-wrap">
            <table className="evidence-table">
              <thead>
                <tr>
                  <th>Job</th>
                  {jobColumns.map(({ label }) => (
                    <th key={label}>{label}</th>
                  ))}
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.resource.uid}>
                    <td>
                      <button onClick={() => onSelect(row.resource)}>{row.resource.name}</button>
                    </td>
                    {jobColumns.map(({ label, value }) => (
                      <td key={label}>{value(row) ?? "Unknown"}</td>
                    ))}
                    <td>{evidenceTime(row.createdAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
      <p className="muted">Deletion, TTL and history limits can remove previously scheduled Jobs.</p>
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load more Jobs</button>}
    </div>
  );
}
