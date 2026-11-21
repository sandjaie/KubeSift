/*
Purpose: Show evidence for the selected Helm revision.
Used by: HelmWorkspace.
Responsibilities: Keep history selection separate from current resource navigation and render stored content inertly.
Important: Stored text is rendered without markup execution.
*/
import { helmActionLabels, type HelmAction } from "./managementState";
import type { CurrentResource, RecordRef } from "../../bindings/internal/helmreader/models";
import { ResourceYaml } from "../workspace/ResourceYaml";
import { evidenceTime } from "../workspace/ResourceOverview";
import { PageControls } from "./HelmWorkspace";
import { sections } from "./state";
import type { HelmController } from "./useHelm";

export function ReleaseDetail({
  helm,
  onOpen,
  onAction,
  canManage,
}: {
  helm: HelmController;
  onOpen: (value: CurrentResource) => void;
  onAction: (action: HelmAction, record?: RecordRef) => void;
  canManage: boolean;
}) {
  const record = helm.revision;
  if (!record)
    return (
      <div className="state-message">
        <h2>Select a release</h2>
        <p>Inspect a retained Helm revision.</p>
      </div>
    );
  const detail = helm.detail?.section;
  const history = helm.detail?.history;
  const overview = detail?.overview;
  return (
    <div className="compact-pod-detail">
      <h2>{record.releaseName}</h2>
      <div className="button-row">
        {(["upgrade", "rollback", "uninstall"] as const).map((action) => (
          <button key={action} disabled={!canManage} onClick={() => onAction(action, record)}>
            {helmActionLabels[action]}
          </button>
        ))}
      </div>
      <p className="detail-identity">
        Revision {record.revision} · {record.namespace} · {record.driver === "secret" ? "Secret" : "ConfigMap"} storage
      </p>
      <div className="detail-tabs" role="tablist" aria-label="Release sections">
        {sections.map((section) => (
          <button key={section} role="tab" aria-selected={helm.section === section} onClick={() => helm.setSection(section)}>
            {section}
          </button>
        ))}
      </div>
      <div className="evidence-section">
        {helm.section === "Values" && (
          <label>
            <input type="checkbox" checked={helm.computed} onChange={(event) => helm.setComputed(event.target.checked)} /> Computed values
            <p className="muted">
              {helm.computed ? "From the stored chart defaults and overrides." : "Stored user overrides."} Values can contain sensitive
              content.
            </p>
          </label>
        )}
        {!helm.detail && <p role="status">Loading revision evidence…</p>}
        {helm.detail?.error && <p role="alert">{helm.detail.error}</p>}
        {overview && (
          <>
            <dl className="pod-facts">
              <dt>Recorded status</dt>
              <dd>{overview.status || "Unknown"}</dd>
              <dt>Chart</dt>
              <dd>{[overview.chartName, overview.chartVersion].filter(Boolean).join(" ") || "Unknown"}</dd>
              <dt>App version</dt>
              <dd>{overview.appVersion || "Unknown"}</dd>
              <dt>First deployed</dt>
              <dd>{overview.firstDeployed ? evidenceTime(overview.firstDeployed) : "Unknown"}</dd>
              <dt>Last deployed</dt>
              <dd>{overview.updatedAt ? evidenceTime(overview.updatedAt) : "Unknown"}</dd>
              {overview.deletedAt && (
                <>
                  <dt>Deleted</dt>
                  <dd>{evidenceTime(overview.deletedAt)}</dd>
                </>
              )}
            </dl>
            {overview.description && <p className="yaml-view">{overview.description}</p>}
          </>
        )}
        {history && (
          <>
            <PageControls page={history.page} total={history.total} onPage={helm.setHistoryPage} label="Revision pages" />
            <table className="resource-table" aria-label="Retained revisions">
              <thead>
                <tr>
                  <th>Revision</th>
                  <th>Status</th>
                  <th>Chart</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {history.items?.map((row, index) => (
                  <tr key={`${row.record.recordName}-${index}`}>
                    <td>
                      {row.problem ? (
                        row.record.revision || "Unknown"
                      ) : (
                        <button aria-pressed={record.uid === row.record.uid} onClick={() => helm.choose(row.record)}>
                          {row.record.revision}
                        </button>
                      )}
                    </td>
                    <td>{row.problem?.message || row.status || "Unknown"}</td>
                    <td>{[row.chartName, row.chartVersion].filter(Boolean).join(" ") || "Unknown"}</td>
                    <td>{row.updatedAt ? evidenceTime(row.updatedAt) : "Unknown"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
        {detail?.section === "Resources" && (
          <>
            <p className="muted">These names came from the stored manifest. A link opens the current object with that name.</p>
            {helm.link?.error && <p role="alert">{helm.link.error}</p>}
            {detail.truncated && <p className="warning">The list reached the resource reference limit.</p>}
            {!detail.resources?.length && <p>No recorded resources.</p>}
            <ul>
              {detail.resources?.map((item, index) => (
                <li key={index}>
                  {item.kind || "Unknown kind"} {item.name || "without a name"} · {item.namespace || "Cluster scope"} ·{" "}
                  {item.apiVersion || "Unknown version"}
                  {item.resolvable ? (
                    <button disabled={helm.link?.pending} onClick={() => helm.openResource(item, onOpen)}>
                      Open current resource
                    </button>
                  ) : (
                    <p className="muted">{item.issue}</p>
                  )}
                </li>
              ))}
            </ul>
          </>
        )}
        {detail &&
          ["Values", "Manifest", "Hooks", "Notes"].includes(detail.section) &&
          (detail.text ? (
            <ResourceYaml stored={{ text: detail.text, label: detail.section.toLowerCase() }} containsConfigData={false} />
          ) : (
            <p>No recorded {detail.section.toLowerCase()}.</p>
          ))}
        {(detail || history) && <p className="muted">Captured {evidenceTime((detail ?? history)!.fetchedAt)}</p>}
      </div>
    </div>
  );
}
