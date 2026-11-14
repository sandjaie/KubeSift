/*
Purpose: Page through locally captured evidence.
Used by: Resource detail sections and the structured resource inspector.
Responsibilities: Render at most 100 entries from the current page.
Important: Paging never fetches from the cluster.
*/
import { useState, type ReactNode } from "react";

export function EvidencePage<T>({
  items,
  render,
  wrap,
}: {
  items: T[];
  render: (item: T, index: number) => ReactNode;
  wrap?: (rows: ReactNode[]) => ReactNode;
}) {
  const [page, setPage] = useState(0);
  const start = Math.min(page, Math.max(0, Math.ceil(items.length / 100) - 1)) * 100;
  const rows = items.slice(start, start + 100).map((item, index) => render(item, start + index));
  return (
    <>
      {wrap ? wrap(rows) : rows}
      {items.length > 100 && (
        <div className="evidence-toolbar evidence-pagination">
          <button disabled={start === 0} onClick={() => setPage(start / 100 - 1)}>
            Previous
          </button>
          <span>
            {start + 1}–{Math.min(start + 100, items.length)} of {items.length}
          </span>
          <button disabled={start + 100 >= items.length} onClick={() => setPage(start / 100 + 1)}>
            Next
          </button>
        </div>
      )}
    </>
  );
}
