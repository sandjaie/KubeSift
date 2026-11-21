/*
Purpose: Show live Pod and Deployment investigation in the existing workspace.
Used by: App for the active connection tab.
Responsibilities: Render bounded page controls and preserve tab-owned navigation.
Important: No synthetic fallback or write action is exposed.
*/
import { useMetrics, metricsIdentity } from "../metrics/useMetrics";
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
import { isStorageKind, storageLabels, type StorageRow } from "../storage/model";
import { StorageTable } from "../storage/StorageTable";
import { isAccessControlKind, accessControlLabels, type AccessControlRow } from "../access-control/model";
import { AccessControlTable } from "../access-control/AccessControlTable";
import { isAvailabilityKind, availabilityLabels, type AvailabilityRow } from "../availability/model";
import { AvailabilityTable } from "../availability/AvailabilityTable";
import { NetworkTable } from "../networking/NetworkTable";
import type { ResourceDetail as CapturedDetail, WorkloadRow } from "../workloads/model";
import { ResourceTable } from "./ResourceTable";
import type { EditTarget, OperationAction, OperationResult } from "../operations/api";
import { ResourceDetail } from "./ResourceDetail";
import { ResizeHandle } from "../ui/ResizeHandle";
import { isClusterKind, clusterLabels, type ClusterResourceRow, type EventInventoryRow } from "../cluster/model";
import { ClusterTable } from "../cluster/ClusterTable";
import { EventInventory } from "../cluster/EventInventory";
import { APIResourceBrowser } from "../discovery/APIResourceBrowser";
export function Workspace({
  tab,
  controller: c,
  navigationWidth,
  navigationOpen,
  collapsedResourceGroups,
  onToggleResourceGroup,
  detailWidthPercent,
  detailHeightPercent,
  onDetailResize,
  onResize,
  onCommitSize,
  logs,
  onOperation,
  onEdit,
  onApply,
  onCreate,
  onInteractive,
  operationResults,
  projectsPane,
  onProjects,
  helmPane,
  onHelm,
  onResourceDestination,
}: {
  tab: ConnectionTab;
  controller: WorkloadController;
  navigationWidth: number;
  navigationOpen: boolean;
  collapsedResourceGroups: readonly string[];
  onToggleResourceGroup: (group: string) => void;
  detailWidthPercent: number;
  detailHeightPercent: number;
  onDetailResize: (axis: "x" | "y", value: number) => void;
  onResize: (n: number) => void;
  onCommitSize: () => void;
  logs?: ReactNode;
  onCreate?: () => void;
  onEdit?: (target: EditTarget, reveal?: boolean) => void;
  onApply?: (target: EditTarget) => void;
  onInteractive?: (kind: "terminal" | "forward", data: CapturedDetail) => void;
  operationResults: OperationResult[];
  projectsPane?: ReactNode;
  onProjects?: () => void;
  helmPane?: ReactNode;
  onHelm: () => void;
  onResourceDestination: () => void;
  onOperation: (action: OperationAction, token: string, acknowledgement: string) => void;
}) {
  const i = c.inventory,
    kind = c.kind;
  const label = isClusterKind(kind)
    ? clusterLabels[kind]
    : isAvailabilityKind(kind)
      ? availabilityLabels[kind]
      : isAccessControlKind(kind)
        ? accessControlLabels[kind]
        : isNetworkKind(kind)
          ? networkLabels[kind]
          : isConfigurationKind(kind)
            ? configurationLabels[kind]
            : isStorageKind(kind)
              ? storageLabels[kind]
              : `${kind}s`;
  const connectionState = visibleState(tab.view);
  const stale = tab.pollFailed || connectionState !== "connected";
  const canReviewOperation = !tab.pollFailed && (connectionState === "connected" || connectionState === "restricted");
  const rows = [...(i.page?.items ?? [])]
    .filter((row) => row.resource.name.toLowerCase().includes(i.filter.toLowerCase()))
    .sort((a, b) => (kind === "Event" ? 0 : a.resource.name.localeCompare(b.resource.name) * (i.sort === "asc" ? 1 : -1)));
  const metrics = useMetrics({
    connection: tab.view?.ref ?? null,
    targets: (i.page?.items ?? []).filter((r) => r.resource.kind === "Pod" || r.resource.kind === "Node").map((r) => r.resource),
    detail:
      c.activeSection === "Overview" && (c.selection?.resource.kind === "Pod" || c.selection?.resource.kind === "Node")
        ? c.selection.resource
        : null,
    visible: !helmPane && (kind === "Pod" || kind === "Node") && canReviewOperation,
    refreshKey: JSON.stringify([i.refresh, i.sectionRefresh]),
  });

  return (
    <div className="workspace live-workspace">
      <div className="explorer">
        {navigationOpen && (
          <>
            <div className="navigation-host">
              <ResourceNavigation
                projects={!!onProjects}
                kind={projectsPane ? "Project" : helmPane ? "Helm" : kind}
                collapsedGroups={collapsedResourceGroups}
                onToggleGroup={onToggleResourceGroup}
                onSelect={(k) => {
                  if (k === "Project") onProjects?.();
                  else if (k === "Helm") onHelm();
                  else if (isResourceKind(k)) {
                    onResourceDestination();
                    c.selectKind(k);
                  }
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
        {projectsPane ? (
          projectsPane
        ) : helmPane ? (
          helmPane
        ) : kind === "APIResources" ? (
          tab.view?.ref && (
            <APIResourceBrowser
              key={JSON.stringify(tab.view.ref)}
              connection={tab.view.ref}
              namespace={tab.namespace}
              route={c.catalogRoute}
              onRoute={c.setCatalogRoute}
              onBack={c.restoreCatalogSource}
              onDedicated={c.selectKind}
              onEdit={onEdit}
              onApply={onApply}
              onCreate={canReviewOperation ? onCreate : undefined}
              operationResults={operationResults}
              widthPercent={detailWidthPercent}
              heightPercent={detailHeightPercent}
              onResize={onDetailResize}
              onCommit={onCommitSize}
            />
          )
        ) : (
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
                {(kind === "Event" || kind === "Lease") && <span className="muted">Namespace: {tab.namespace}</span>}
                {kind === "Secret" && <span className="muted">Metadata only</span>}
                <span className="muted">{rows.length} shown</span>
                <button
                  disabled={!canReviewOperation || !onCreate}
                  onClick={(event) => {
                    event.currentTarget.focus();
                    onCreate?.();
                  }}
                >
                  Create resource…
                </button>
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
                {kind !== "Event" && (
                  <button onClick={() => c.patch({ sort: i.sort === "asc" ? "desc" : "asc" })}>Name {i.sort === "asc" ? "↑" : "↓"}</button>
                )}
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
              <p className="table-hint">
                ↑ ↓ Select · {kind === "Event" ? "Newest observed first within this page only." : "Filter and sort apply to this page."}
              </p>
              {i.loading && <p>Loading {label}…</p>}
              {i.error && <p role="alert">{i.error}</p>}
              {rows.length ? (
                kind === "Event" ? (
                  <EventInventory
                    rows={rows as EventInventoryRow[]}
                    selected={c.selection?.resource.uid ?? null}
                    onSelect={c.selectResource}
                  />
                ) : isClusterKind(kind) ? (
                  <ClusterTable
                    metrics={metrics.byIdentity}
                    kind={kind}
                    rows={rows as ClusterResourceRow[]}
                    selected={c.selection?.resource.uid ?? null}
                    onSelect={c.selectResource}
                  />
                ) : isAvailabilityKind(kind) ? (
                  <AvailabilityTable
                    rows={rows as AvailabilityRow[]}
                    kind={kind}
                    selected={c.selection?.resource.uid ?? null}
                    onSelect={c.selectResource}
                  />
                ) : isAccessControlKind(kind) ? (
                  <AccessControlTable
                    rows={rows as AccessControlRow[]}
                    kind={kind}
                    selected={c.selection?.resource.uid ?? null}
                    onSelect={c.selectResource}
                  />
                ) : isStorageKind(kind) ? (
                  <StorageTable
                    rows={rows as StorageRow[]}
                    kind={kind}
                    selected={c.selection?.resource.uid ?? null}
                    onSelect={c.selectResource}
                  />
                ) : isNetworkKind(kind) ? (
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
                    metrics={metrics.byIdentity}
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
                <ResourceDetail
                  usage={metrics.byIdentity[metricsIdentity(c.selection.resource)]}
                  controller={c}
                  context={tab.entry.context}
                  stale={stale}
                  canReviewOperation={canReviewOperation}
                  logs={logs}
                  onOperation={onOperation}
                  onEdit={onEdit}
                  onApply={onApply}
                  onInteractive={onInteractive}
                />
              ) : (
                <div className="state-message">
                  <h2>Select a {kind}</h2>
                  <p>Inspect captured resource evidence.</p>
                </div>
              )}
            </section>
          </ResourceSplit>
        )}
      </div>
    </div>
  );
}
