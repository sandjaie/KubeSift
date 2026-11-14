/*
Purpose: Show Pods associated with one captured PDB selector.
Used by: The Selected Pods investigation tab.
Responsibilities: Display the scope, source version and continuation of the association.
Important: Matches do not establish ownership or protection.
*/
import type { ResourceRef } from "../workloads/model";
import { selectorLabel, type SelectedPodPage } from "./model";
import { PodPhase } from "../workspace/PodPhase";

export function SelectedPods({
  data,
  onNext,
  onSelect,
}: {
  data: SelectedPodPage;
  onNext: (cursor: string) => void;
  onSelect: (target: ResourceRef) => void;
}) {
  const rows = data.items ?? [];
  return (
    <div className="evidence-section">
      <p className="muted">
        {selectorLabel(JSON.parse(data.selectorJSON))} · source resourceVersion {data.sourceVersion}
      </p>
      <details>
        <summary>Captured selector</summary>
        <pre className="network-value">{data.selectorJSON}</pre>
      </details>
      <p className="muted">Selector matches show label association. They do not establish ownership or eviction eligibility.</p>
      {data.problem ? (
        <p role="alert">{data.problem.message}</p>
      ) : (
        <>
          <p>
            {rows.length
              ? `${rows.length} matches on this page`
              : data.complete
                ? "No selector matches found."
                : "No matches on this page. More results may be available."}
          </p>
          {!!rows.length && (
            <div className="evidence-table-wrap">
              <table className="evidence-table" aria-label="Selected Pods">
                <thead>
                  <tr>
                    <th>Pod</th>
                    <th>Phase</th>
                    <th>Association</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr key={row.resource.uid}>
                      <td>
                        <button className="network-value" onClick={() => onSelect(row.resource)}>
                          {row.resource.name}
                        </button>
                      </td>
                      <td>
                        <PodPhase phase={row.phase} />
                      </td>
                      <td>Selector match</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Load more selected Pods</button>}
    </div>
  );
}
