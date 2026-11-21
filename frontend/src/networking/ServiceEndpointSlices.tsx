/*
Purpose: Show one batch of slices associated with a selected Service.
Used by: Service EndpointSlices detail tab.
Responsibilities: Explain owner evidence and prevent navigation through conflicts.
Important: An empty batch does not establish that a Service cannot route.
*/
import type { ResourceRef } from "../workloads/model";
import type { InventoryProjection, ServiceSlicePage } from "./model";
import { managerLabel } from "./model";

export function ServiceEndpointSlices({
  data,
  onNext,
  onSelect,
}: {
  data: ServiceSlicePage;
  onNext: (cursor: string) => void;
  onSelect: (resource: ResourceRef) => void;
}) {
  const rows = data.items ?? [];
  return (
    <div className="evidence-section">
      {data.problem && <p role="alert">{data.problem.message}</p>}
      {!data.problem && (
        <p>
          {rows.length
            ? `${rows.length} associated slices shown in this batch`
            : data.complete
              ? "No label-associated slices found."
              : "No associated slices in this batch. More results may be available."}
        </p>
      )}
      {rows.length > 0 && (
        <div className="evidence-table-wrap">
          <table className="evidence-table">
            <thead>
              <tr>
                <th>Slice</th>
                <th>Address type</th>
                <th>Endpoints</th>
                <th>Association</th>
                <th>Managed by</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ slice, association }) => {
                const o = JSON.parse(slice.objectJSON) as InventoryProjection["EndpointSlice"];
                const evidence =
                  association === "owner-confirmed"
                    ? "Label-associated; Service owner UID matches"
                    : association === "owner-conflict"
                      ? "Relationship conflict: Service owner differs"
                      : "Label-associated; ownership unverified";
                return (
                  <tr key={slice.resource.uid}>
                    <td>
                      {association === "owner-conflict" ? (
                        slice.resource.name
                      ) : (
                        <button onClick={() => onSelect(slice.resource)}>{slice.resource.name}</button>
                      )}
                    </td>
                    <td>{o.addressType ?? "Unknown"}</td>
                    <td>{o.endpointCount ?? "Unknown"}</td>
                    <td className="network-value">{evidence}</td>
                    <td className="network-value">{o.metadata?.labels?.[managerLabel] ?? "Not specified"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <p className="muted">The Service name label associates slices. This evidence does not establish reachability.</p>
      {data.page.nextCursor && <button onClick={() => onNext(data.page.nextCursor)}>Next batch</button>}
    </div>
  );
}
