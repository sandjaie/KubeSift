/*
Purpose: Present UID-scoped Kubernetes event evidence.
Used by: Workload Events sections.
Responsibilities: Label page-local ordering and optional timestamps without diagnosing absence.
Important: Empty event history does not establish that no failure occurred.
*/
import { useState } from "react";
import type { EventPage } from "../workloads/model";
import { evidenceTime } from "./ResourceOverview";

export function ResourceEvents({ data, onNext }: { data: EventPage; onNext: (cursor: string) => void }) {
  const [warningsOnly, setWarningsOnly] = useState(false);
  const items = data.items ?? [];
  const warnings = items.filter((event) => event.type === "Warning");
  const shown = warningsOnly ? warnings : items;
  return (
    <div className="evidence-section">
      <div className="filter-bar" aria-label="Filter events on this page">
        <button aria-pressed={!warningsOnly} onClick={() => setWarningsOnly(false)}>
          All ({items.length})
        </button>
        <button aria-pressed={warningsOnly} onClick={() => setWarningsOnly(true)}>
          Warnings ({warnings.length})
        </button>
        <span className="muted">This page</span>
      </div>
      {shown.length ? (
        <div className="evidence-table-wrap">
          <table className="evidence-table">
            <thead>
              <tr>
                <th>Type</th>
                <th>Reason / message</th>
                <th>Count</th>
                <th>Last seen</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((event) => (
                <tr key={event.uid}>
                  <td className={event.type === "Warning" ? "warning" : undefined}>
                    {event.type === "Warning" && <span aria-hidden="true">⚠ </span>}
                    {event.type || "Unknown"}
                  </td>
                  <td>
                    <details>
                      <summary>
                        <strong>{event.reason || "Unknown"}</strong>
                        <br />
                        {event.message.slice(0, 120)}
                        {event.message.length > 120 ? "…" : ""}
                      </summary>
                      <p>{event.message || "Unknown message"}</p>
                      <p className="muted">
                        Reporter {event.reporter || "Unknown"} · First {event.firstAt ?? "Unknown"} · Last {event.lastAt ?? "Unknown"}
                      </p>
                    </details>
                  </td>
                  <td>{event.count ?? "Unknown"}</td>
                  <td>{evidenceTime(event.lastAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p>{warningsOnly ? "No warnings on this page." : "No events returned. Retention and permissions can leave gaps."}</p>
      )}
      <p className="muted">Newest first on this page · Captured {evidenceTime(data.page.fetchedAt)}</p>
      {data.page.nextCursor ? (
        <button onClick={() => onNext(data.page.nextCursor)}>Next events page</button>
      ) : (
        <span className="muted">End of results</span>
      )}
    </div>
  );
}
