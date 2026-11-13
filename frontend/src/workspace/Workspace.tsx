/*
Purpose: Show live Pod and Deployment investigation in the existing workspace.
Used by: App for the active connection tab.
Responsibilities: Render bounded page controls and preserve tab-owned navigation.
Important: No synthetic fallback or write action is exposed.
*/
import { ResourceSplit } from "./ResourceSplit";
import { isResourceKind } from "../workloads/model";
import type { ReactNode } from "react";
import type { WorkloadController } from "../workloads/useWorkloads";
import type { ConnectionTab } from "../connections/model";
import { visibleState } from "../connections/state";
import { ResourceNavigation } from "./ResourceNavigation";
import { isNetworkKind, networkLabels, type NetworkRow } from "../networking/model";
import { ConfigurationTable } from "../configuration/ConfigurationTable";
import { configurationLabels, isConfigurationKind, type ConfigurationRow } from "../configuration/model";
import { NetworkTable } from "../networking/NetworkTable";
import type { WorkloadRow } from "../workloads/model";
import { ResourceTable } from "./ResourceTable";
import { ResourceDetail } from "./ResourceDetail";
import { ResizeHandle } from "../ui/ResizeHandle";
export function Workspace({
  tab,
  controller: c,
  navigationWidth,
  navigationOpen,
  detailWidthPercent,
  detailHeightPercent,
  onDetailResize,
  onResize,
  onCommitSize,
  logs,
}: {
  tab: ConnectionTab;
  controller: WorkloadController;
  navigationWidth: number;
  navigationOpen: boolean;
  detailWidthPercent: number;
  detailHeightPercent: number;
  onDetailResize: (axis: "x" | "y", value: number) => void;
  onResize: (n: number) => void;
  onCommitSize: () => void;
  logs?: ReactNode;
}) {
  const i = c.inventory,
    kind = c.kind;
  const label = isNetworkKind(kind) ? networkLabels[kind] : isConfigurationKind(kind) ? configurationLabels[kind] : `${kind}s`;
  const stale = tab.pollFailed || visibleState(tab.view) !== "connected";
  const rows = [...(i.page?.items ?? [])]
    .filter((row) => row.resource.name.toLowerCase().includes(i.filter.toLowerCase()))
    .sort((a, b) => a.resource.name.localeCompare(b.resource.name) * (i.sort === "asc" ? 1 : -1));
  return (
    <div className="workspace live-workspace">
      <div className="explorer">
        {navigationOpen && (
          <>
            <div className="navigation-host">
              <ResourceNavigation
                kind={kind}
                onSelect={(k) => {
                  if (isResourceKind(k)) c.selectKind(k);
                }}
              />
            </div>
            <ResizeHandle
              label="Navigation width"
              value={navigationWidth}
              min={160}
              max={320}
              onChange={onResize}
              onCommit={onCommitSize}
            />
          </>
        )}
        <ResourceSplit
          widthPercent={detailWidthPercent}
          heightPercent={detailHeightPercent}
          onResize={onDetailResize}
          onCommit={onCommitSize}
        >
          <section
            className="list-pane"
            onScroll={(e) => c.patch({ scroll: e.currentTarget.scrollTop })}
            ref={(node) => {
              if (node && Math.abs(node.scrollTop - i.scroll) > 2) node.scrollTop = i.scroll;
            }}
          >
            <div className="pane-heading">
              <h2>{label}</h2>
              {c.clusterScoped && <span className="muted">Cluster-scoped</span>}
              {kind === "Secret" && <span className="muted">Metadata only</span>}
              <span className="muted">{rows.length} shown</span>
              <button disabled={!tab.view?.ref || i.loading} onClick={c.refresh}>
                ↻ Refresh
              </button>
            </div>
            <div className="filter-bar">
              <input
                spellCheck={false}
                autoCorrect="off"
                autoCapitalize="none"
                autoComplete="off"
                aria-label="Filter this page"
                placeholder="Filter this page…"
                value={i.filter}
                onChange={(e) => c.patch({ filter: e.target.value })}
              />
              <button onClick={() => c.patch({ sort: i.sort === "asc" ? "desc" : "asc" })}>Name {i.sort === "asc" ? "↑" : "↓"}</button>
            </div>
            <div className="inventory-controls">
              <nav className="inventory-pagination" aria-label="Resource pages">
                <button disabled={i.history.length === 1 || i.loading} onClick={c.previous}>
                  Previous
                </button>
                <span className="inventory-page">Page {i.history.length}</span>
                <button disabled={!i.page?.page.nextCursor || i.history.length >= 20 || i.loading} onClick={c.next}>
                  Next
                </button>
              </nav>
              <label className="inventory-auto-refresh">
                <input type="checkbox" checked={i.auto} onChange={(e) => c.patch({ auto: e.target.checked })} /> Auto-refresh
              </label>
            </div>
            {i.history.length >= 20 && <p className="warning">History limit reached. Refresh to restart pagination.</p>}
            {i.auto && i.history.length > 1 && <p className="muted">Auto-refresh paused after page one.</p>}
            {stale && i.page && <p className="warning table-hint">Retained data — connection evidence is not current.</p>}
            <p className="table-hint">↑ ↓ Select · Filter and sort apply to this page.</p>
            {i.loading && <p>Loading {label}…</p>}
            {i.error && <p role="alert">{i.error}</p>}
            {rows.length ? (
              isNetworkKind(kind) ? (
                <NetworkTable
                  rows={rows as NetworkRow[]}
                  kind={kind}
                  selected={c.selection?.resource.uid ?? null}
                  onSelect={c.selectResource}
                />
              ) : isConfigurationKind(kind) ? (
                <ConfigurationTable
                  rows={rows as ConfigurationRow[]}
                  kind={kind}
                  selected={c.selection?.resource.uid ?? null}
                  onSelect={c.selectResource}
                />
              ) : (
                <ResourceTable
                  rows={rows as WorkloadRow[]}
                  kind={kind}
                  selected={c.selection?.resource.uid ?? null}
                  onSelect={c.selectResource}
                />
              )
            ) : (
              !i.loading && (
                <p className="state-message">
                  {i.filter
                    ? "No matches on this page."
                    : i.page
                      ? "No resources returned on this page."
                      : "Connect or Retry to read resources."}
                </p>
              )
            )}
          </section>
          <section className="detail-pane">
            {c.selection ? (
              <ResourceDetail controller={c} context={tab.entry.context} stale={stale} logs={logs} />
            ) : (
              <div className="state-message">
                <h2>Select a {kind}</h2>
                <p>Inspect captured resource evidence.</p>
              </div>
            )}
          </section>
        </ResourceSplit>
      </div>
    </div>
  );
}
