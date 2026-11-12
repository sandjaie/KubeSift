/*
Purpose: Present UID-scoped Kubernetes event evidence.
Used by: Workload Events sections.
Responsibilities: Label page-local ordering and optional timestamps without diagnosing absence.
Important: Empty event history does not establish that no failure occurred.
*/
import type { EventPage } from "../workloads/model";
export function ResourceEvents({ data, onNext }: { data: EventPage; onNext: (cursor: string) => void }) {
  return (
    <div className="evidence-section">
      <p>Captured {data.page.fetchedAt}. Sorted newest first within this page; not a globally newest guarantee.</p>
      {!data.items?.length && <p>No events returned. Retention and permissions can leave gaps.</p>}
      {data.items?.map((e) => (
        <article className="event-evidence" key={e.uid}>
          <strong>
            {e.type} · {e.reason}
          </strong>
          <p>{e.message}</p>
          <p className="muted">
            {e.reporter || "Unknown reporter"} · Count {e.count ?? "Unknown"} · First {e.firstAt ?? "Unknown"} · Last{" "}
            {e.lastAt ?? "Unknown"}
          </p>
        </article>
      ))}
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Next events page</button>}
    </div>
  );
}
