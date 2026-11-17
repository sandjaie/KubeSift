/*
Purpose: Show namespace Helm releases in the existing list and detail split.
Used by: The Helm destination in Workspace.
Responsibilities: Select storage, filter the bounded inventory, page and select a release revision.
Important: Records show stored status for the selected storage driver.
*/
import { useState } from "react";
import type { CurrentResource } from "../../bindings/internal/helmreader/models";
import { ResourceSplit } from "../workspace/ResourceSplit";
import { evidenceTime } from "../workspace/ResourceOverview";
import { ReleaseDetail } from "./ReleaseDetail";
import type { HelmController } from "./useHelm";

export function HelmWorkspace({
  helm,
  namespace,
  onOpen,
  widthPercent,
  heightPercent,
  onResize,
  onCommit,
}: {
  helm: HelmController;
  namespace: string;
  onOpen: (resource: CurrentResource) => void;
  widthPercent: number;
  heightPercent: number;
  onResize: (axis: "x" | "y", value: number) => void;
  onCommit: () => void;
}) {
  const [descending, setDescending] = useState(false);
  const rows = helm.list?.items ?? [];
  return (
    <ResourceSplit widthPercent={widthPercent} heightPercent={heightPercent} onResize={onResize} onCommit={onCommit}>
      <section className="list-pane">
        <div className="pane-heading">
          <h2>Helm releases</h2>
          <span className="muted">Namespace: {namespace}</span>
          <button disabled={helm.loadingList} onClick={helm.refresh}>
            ↻ Refresh
          </button>
        </div>
        <form
          className="filter-bar"
          onSubmit={(event) => {
            event.preventDefault();
            helm.applyFilter();
          }}
        >
          <label>
            Storage{" "}
            <select value={helm.driver} onChange={(event) => helm.setDriver(event.target.value === "configmap" ? "configmap" : "secret")}>
              <option value="secret">Secrets</option>
              <option value="configmap">ConfigMaps</option>
            </select>
          </label>
          <input
            aria-label="Exact release name"
            placeholder="Exact release name"
            spellCheck={false}
            autoComplete="off"
            value={helm.filter}
            onChange={(event) => helm.setFilter(event.target.value)}
          />
          <button type="submit">Filter</button>
        </form>
        {helm.list && (
          <div className="inventory-controls">
            <PageControls page={helm.list.page} total={helm.list.total} onPage={helm.changePage} label="Release pages" />
            <button onClick={() => setDescending(!descending)}>Name {descending ? "↓" : "↑"}</button>
            <span className="muted">{helm.list.total} in snapshot</span>
          </div>
        )}
        {helm.loadingList && <p role="status">Loading Helm releases…</p>}
        {helm.listError && <p role="alert">{helm.listError}</p>}
        {helm.list && rows.length === 0 && <p>No matching Helm releases in {helm.driver === "secret" ? "Secret" : "ConfigMap"} storage.</p>}
        {rows.length > 0 && (
          <table className="resource-table" aria-label="Helm releases">
            <thead>
              <tr>
                <th>Name</th>
                <th>Revision</th>
                <th>Status</th>
                <th>Chart</th>
                <th>App</th>
                <th>Updated</th>
              </tr>
            </thead>
            <tbody>
              {(descending ? [...rows].reverse() : rows).map((row, index) => (
                <tr key={`${row.record.recordName}-${index}`}>
                  <td>
                    {row.problem ? (
                      row.record.releaseName || "Unknown release"
                    ) : (
                      <button aria-pressed={helm.revision?.releaseName === row.record.releaseName} onClick={() => helm.choose(row.record)}>
                        {row.record.releaseName}
                      </button>
                    )}
                  </td>
                  <td>{row.record.revision || "Unknown"}</td>
                  <td>{row.problem?.message || row.status || "Unknown"}</td>
                  <td>{[row.chartName, row.chartVersion].filter(Boolean).join(" ") || "Unknown"}</td>
                  <td>{row.appVersion || "Unknown"}</td>
                  <td>{row.updatedAt ? evidenceTime(row.updatedAt) : "Unknown"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {helm.list && <p className="table-hint">Name ordering applies to this page. Captured {evidenceTime(helm.list.fetchedAt)}.</p>}
      </section>
      <section className="detail-pane">
        <ReleaseDetail helm={helm} onOpen={onOpen} />
      </section>
    </ResourceSplit>
  );
}

export function PageControls({
  page,
  total,
  label,
  onPage,
}: {
  page: number;
  total: number;
  label: string;
  onPage: (page: number) => void;
}) {
  return (
    <nav className="inventory-pagination" aria-label={label}>
      <button disabled={page === 0} onClick={() => onPage(page - 1)}>
        Previous
      </button>
      <span className="inventory-page">Page {page + 1}</span>
      <button disabled={(page + 1) * 100 >= total} onClick={() => onPage(page + 1)}>
        Next
      </button>
    </nav>
  );
}
