/*
Purpose: Present live workload investigation in the approved detail pane.
Used by: Workspace after explicit resource selection.
Responsibilities: Keep section failures independent and render cluster fields as inert text.
Important: Overview configuration is not measured usage and references never fetch Secret values.
*/
import { HistoricalUsage } from "../metrics/HistoricalUsage";
import { CurrentUsage } from "../metrics/MetricsView";
import type { SampleView } from "../metrics/useMetrics";
import { useId, useLayoutEffect, useState, type ReactNode } from "react";
import { NodeActionDialog, type NodeAction, nodeLabel } from "../operations/NodeActionDialog";
import { ScaleDialog } from "../operations/ScaleDialog";
import { DeleteDialog } from "../operations/DeleteDialog";
import { deleteUnavailable } from "../operations/deleteState";
import { RestartDialog } from "../operations/RestartDialog";
import { restartUnavailable } from "../operations/restartState";
import type { OperationAction } from "../operations/api";
import type { WorkloadController } from "../workloads/useWorkloads";
import type {
  ResourceDetail as Detail,
  YAMLDocument,
  EventPage,
  RelatedPage,
  RelatedReplicaSetPage,
  RelatedJobPage,
} from "../workloads/model";
import { isClusterScoped, isWorkloadKind, resourceSections } from "../workloads/model";
import { isConfigurationKind } from "../configuration/model";
import { ConfigurationOverview } from "../configuration/ConfigurationOverview";
import { ConfigMapData } from "../configuration/ConfigMapData";
import { PodConfiguration } from "../configuration/PodConfiguration";
import { isNetworkKind, type ServiceSlicePage } from "../networking/model";
import { isStorageKind } from "../storage/model";
import { StorageOverview } from "../storage/StorageOverview";
import { PodStorage } from "../storage/PodStorage";
import { isAccessControlKind } from "../access-control/model";
import { AccessControlOverview } from "../access-control/AccessControlOverview";
import { PodIdentity } from "../access-control/PodIdentity";
import { isAvailabilityKind, type SelectedPodPage, type RolloutHistoryPage } from "../availability/model";
import { AvailabilityOverview } from "../availability/AvailabilityOverview";
import { SelectedPods } from "../availability/SelectedPods";
import { ControllerRevisions } from "../availability/ControllerRevisions";
import { NetworkOverview } from "../networking/NetworkOverview";
import { ServiceEndpointSlices } from "../networking/ServiceEndpointSlices";
import { fixedEditTarget } from "../operations/editState";
import type { EditTarget } from "../operations/api";
import { ResourceYaml } from "./ResourceYaml";
import { ResourceEvents } from "./ResourceEvents";
import { DeploymentPods } from "./DeploymentPods";
import { RelatedJobs } from "./RelatedJobs";
import { ReplicaSetList } from "./ReplicaSetList";
import { ResourceOverview, object, list, overviewObject } from "./ResourceOverview";
import { isClusterKind } from "../cluster/model";
import { ClusterOverview } from "../cluster/ClusterOverview";
export function ResourceDetail({
  usage,
  controller: c,
  context,
  stale,
  canReviewOperation,
  logs,
  onOperation,
  onEdit,
  onApply,
  onInteractive,
}: {
  usage?: SampleView;
  controller: WorkloadController;
  context: string;
  stale: boolean;
  canReviewOperation: boolean;
  logs?: ReactNode;
  onEdit?: (target: EditTarget, reveal?: boolean) => void;
  onApply?: (target: EditTarget) => void;
  onInteractive?: (kind: "terminal" | "forward", data: Detail) => void;
  onOperation: (action: OperationAction, token: string, acknowledgement: string) => void;
}) {
  const [review, setReview] = useState<{ action: OperationAction; key: string }>();
  const restartReasonID = useId();
  const deleteReasonID = useId();
  const row = c.selection;
  const deletionAccepted = c.inventory.retainedAfterDeletion;

  const section = c.activeSection,
    state = c.sections[section],
    data = state?.data;
  const kind = row?.resource.kind ?? "";
  const tabs = resourceSections(kind);
  const link = c.inventory.referenceEvidence;
  const capturedLink =
    link &&
    section === "Overview" &&
    !state?.loading &&
    !state?.error &&
    (data as Detail | undefined)?.resourceVersion === link.targetVersion;
  const supportsScale = kind === "Deployment" || kind === "StatefulSet";
  const supportsRestart = supportsScale || kind === "DaemonSet";
  const source =
    section === "Overview" && canReviewOperation && !state?.loading && !state?.error && isWorkloadKind(kind)
      ? (data as Detail | undefined)
      : undefined;
  const sourceKey = source?.resourceVersion
    ? JSON.stringify([source.resource, source.resourceVersion, c.inventory.refresh, c.inventory.sectionRefresh])
    : "";
  const interactiveCapture =
    section === "Overview" && canReviewOperation && !state?.loading && !state?.error && (kind === "Pod" || kind === "Service")
      ? (data as Detail | undefined)
      : undefined;
  const nodeSource =
    kind === "Node" &&
    section === "Overview" &&
    !stale &&
    canReviewOperation &&
    !state?.loading &&
    !state?.error &&
    !!(data as Detail)?.resourceVersion &&
    (data as Detail).resource.name === row?.resource.name &&
    (data as Detail).resource.uid === row?.resource.uid &&
    (data as Detail).resource.connection.id === row?.resource.connection.id &&
    (data as Detail).resource.connection.generation === row?.resource.connection.generation
      ? (data as Detail)
      : undefined;
  const nodeKey = nodeSource
    ? JSON.stringify([nodeSource.resource, nodeSource.resourceVersion, c.inventory.refresh, c.inventory.sectionRefresh])
    : "";
  const [nodeReview, setNodeReview] = useState<{ action: NodeAction; key: string }>();
  useLayoutEffect(() => setNodeReview(undefined), [nodeKey]);
  const scheduling = object(overviewObject(nodeSource).spec).unschedulable === true ? "uncordon" : "cordon";
  const groupedActions = kind === "Deployment" || kind === "Pod";
  const podOverview = c.sections.Overview;
  const terminalTarget =
    kind === "Pod" && canReviewOperation && !podOverview?.error && !podOverview?.loading
      ? (podOverview?.data as Detail | undefined)
      : undefined;
  const restartReason = source && supportsRestart ? restartUnavailable(source) : "";
  const deleteReason = source ? deleteUnavailable(source) : "";
  useLayoutEffect(() => {
    setReview(undefined);
  }, [sourceKey]);
  if (!row) return null;
  return (
    <div className={section === "YAML" ? "compact-pod-detail yaml-detail" : "compact-pod-detail"}>
      {deletionAccepted && <button onClick={c.backToInventory}>← Back to inventory</button>}
      {c.back && (
        <button onClick={c.backToResource}>
          ← Back to {c.back.resource.kind} {c.back.resource.name}
        </button>
      )}
      {c.navigationError && <p role="alert">{c.navigationError}</p>}
      <div className="pane-heading">
        <h2>{row.resource.name}</h2>
        <div className="scale-actions">
          {nodeSource && (
            <select
              aria-label="Node actions"
              value=""
              onChange={(e) => {
                const action = e.target.value;
                if (action === scheduling || action === "drain") setNodeReview({ action, key: nodeKey });
              }}
            >
              <option value="" disabled>
                Actions
              </option>
              <option value={scheduling}>{nodeLabel[scheduling]}…</option>
              <option value="drain">Drain…</option>
            </select>
          )}
          {nodeSource && nodeReview?.key === nodeKey && (
            <NodeActionDialog
              action={nodeReview.action}
              source={nodeSource}
              cluster={context}
              onDismiss={() => setNodeReview(undefined)}
              onExecute={(token, ack) => {
                setNodeReview(undefined);
                onOperation("node", token, ack);
              }}
            />
          )}
          {groupedActions && source && (
            <select
              aria-label={kind + " actions"}
              value=""
              onChange={(event) => {
                const action = event.target.value;
                event.currentTarget.focus();
                if (action === "forward" && interactiveCapture) onInteractive?.("forward", interactiveCapture);
                if (action === "scale" || action === "restart" || action === "delete") setReview({ action, key: sourceKey });
              }}
            >
              <option value="" disabled>
                Actions
              </option>
              {supportsScale && <option value="scale">Scale…</option>}
              {supportsRestart && (
                <option value="restart" disabled={!!restartReason}>
                  Restart…
                </option>
              )}
              {onInteractive && interactiveCapture && <option value="forward">Port forward…</option>}
              <option value="delete" disabled={!!deleteReason || deletionAccepted}>
                Delete…
              </option>
            </select>
          )}
          {!groupedActions && supportsScale && sourceKey && (
            <button
              onClick={(event) => {
                event.currentTarget.focus();
                setReview({ action: "scale", key: sourceKey });
              }}
            >
              Scale…
            </button>
          )}
          {!groupedActions && source && supportsRestart && (
            <button
              disabled={!!restartReason}
              aria-describedby={restartReason ? restartReasonID : undefined}
              onClick={(event) => {
                event.currentTarget.focus();
                setReview({ action: "restart", key: sourceKey });
              }}
            >
              Restart…
            </button>
          )}
          {!groupedActions && source && (
            <button
              className="danger"
              disabled={!!deleteReason || deletionAccepted}
              aria-describedby={deleteReason ? deleteReasonID : undefined}
              onClick={(event) => {
                event.currentTarget.focus();
                setReview({ action: "delete", key: sourceKey });
              }}
            >
              Delete…
            </button>
          )}
          {kind === "Service" && onInteractive && interactiveCapture && (
            <button onClick={() => onInteractive("forward", interactiveCapture)}>Port forward…</button>
          )}
          {section !== "Logs" && (
            <button onClick={c.refreshSection} disabled={state?.loading}>
              Refresh section
            </button>
          )}
        </div>
      </div>
      {deletionAccepted && <p role="status">Kubernetes accepted deletion. Removal may still be in progress.</p>}
      {source && deleteReason && (
        <p className="muted" id={deleteReasonID}>
          {deleteReason}
        </p>
      )}
      {source && restartReason && (
        <p className="muted" id={restartReasonID}>
          {restartReason}
        </p>
      )}
      {source &&
        sourceKey &&
        review?.key === sourceKey &&
        (review.action === "scale" ? (
          <ScaleDialog
            key={sourceKey}
            source={source}
            cluster={context}
            onDismiss={() => setReview(undefined)}
            onExecute={(result, ack) => onOperation("scale", result.token, ack)}
          />
        ) : review.action === "delete" ? (
          !deleteReason &&
          !deletionAccepted && (
            <DeleteDialog
              key={sourceKey}
              source={source}
              cluster={context}
              onDismiss={() => setReview(undefined)}
              onExecute={(result, ack) => onOperation("delete", result.token, ack)}
            />
          )
        ) : (
          !restartReason && (
            <RestartDialog
              key={sourceKey}
              source={source}
              cluster={context}
              onDismiss={() => setReview(undefined)}
              onExecute={(result, ack) => onOperation("restart", result.token, ack)}
            />
          )
        ))}
      <p className="detail-identity">
        {context} / {isClusterScoped(kind) ? "Cluster-scoped" : row.resource.namespace} / {row.resource.kind}
      </p>
      {c.inventory.identity && <p className="muted">{c.inventory.identity}</p>}
      {link && (
        <div className="muted" title={`Source version ${link.sourceVersion}; target version ${link.targetVersion}`}>
          <p>{link.identityEvidence === "uid-verified" ? "Referenced UID verified" : "Resolved by name at open time"}</p>
          {link.association && (
            <p>
              {link.association === "selector-match"
                ? "Selector match · Label association"
                : link.association === "controller-owner"
                  ? "Controller owner verified"
                  : "Explicit scale target"}
            </p>
          )}
          {capturedLink && link.bindingEvidence && link.bindingEvidence !== "not-applicable" && (
            <p>
              {link.bindingEvidence === "references-agree" ? "Binding references agree" : "Reference only; binding unconfirmed"} · Observed
              in separate reads.
            </p>
          )}
        </div>
      )}
      {stale && <p className="warning">Retained evidence — connection status is stale.</p>}
      <div className="detail-tabs">
        <div className="resource-section-tabs" role="tablist">
          {tabs.map((tab) => (
            <button key={tab} role="tab" aria-selected={tab === section} onClick={() => c.selectSection(tab)}>
              {tab}
            </button>
          ))}
        </div>
        {kind === "Pod" && onInteractive && (
          <button disabled={!terminalTarget} onClick={() => terminalTarget && onInteractive("terminal", terminalTarget)}>
            Terminal
          </button>
        )}
      </div>
      {state?.loading && <p>Loading {section}…</p>}
      {state?.error && (
        <p role="alert">
          {state.error} {data ? "Retained section evidence may be stale." : ""}
        </p>
      )}
      {section === "Overview" && (kind === "Pod" || kind === "Node") && <CurrentUsage view={usage} />}
      {section === "Overview" && canReviewOperation && !state?.error && data && (kind === "Pod" || kind === "Node") && (
        <HistoricalUsage
          key={JSON.stringify(row.resource)}
          resource={row.resource}
          connection={context}
          containers={["containers", "initContainers", "ephemeralContainers"].flatMap((k) =>
            list(object(overviewObject(data as Detail).spec)[k])
              .map((v) => object(v).name)
              .filter((n): n is string => typeof n === "string"),
          )}
        />
      )}

      {section === "Overview" &&
        data &&
        (isClusterKind(kind) ? (
          <ClusterOverview
            key={JSON.stringify([row.resource.uid, (data as Detail).resourceVersion])}
            data={data as Detail}
            onTarget={() => c.openEventTarget(data as Detail)}
            onBrowse={(intent) => c.browseCatalog(data as Detail, intent)}
          />
        ) : isAvailabilityKind(kind) ? (
          <AvailabilityOverview data={data as Detail} onOpenReference={(target) => c.openAvailabilityReference(data as Detail, target)} />
        ) : isAccessControlKind(kind) ? (
          <AccessControlOverview data={data as Detail} onOpenReference={(target) => c.openAccessControlReference(data as Detail, target)} />
        ) : isStorageKind(kind) ? (
          <StorageOverview data={data as Detail} onOpenReference={(target) => c.openStorageReference(data as Detail, target)} />
        ) : isNetworkKind(kind) ? (
          <NetworkOverview data={data as Detail} onOpenReference={(target) => c.openReference(data as Detail, target)} />
        ) : isConfigurationKind(kind) ? (
          <ConfigurationOverview
            data={data as Detail}
            onBrowseNamespace={(destination) => c.browseNamespace(data as Detail, destination)}
          />
        ) : (
          <>
            <ResourceOverview data={data as Detail} onOpenOwner={c.openRelated} />
            {kind === "Pod" && (
              <>
                <PodIdentity data={data as Detail} onOpenReference={(target) => c.openAccessControlReference(data as Detail, target)} />
                <PodStorage data={data as Detail} onOpenReference={(target) => c.openStorageReference(data as Detail, target)} />
                <PodConfiguration data={data as Detail} onOpen={(target) => c.openConfigurationReference(data as Detail, target)} />
              </>
            )}
          </>
        ))}
      {section === "Data" && kind === "ConfigMap" && data && (
        <ConfigMapData key={JSON.stringify([row.resource.uid, (data as Detail).resourceVersion])} data={data as Detail} />
      )}
      {section === "EndpointSlices" && data && (
        <ServiceEndpointSlices
          data={data as ServiceSlicePage}
          onNext={c.nextSection}
          onSelect={(target) => c.openServiceSlice((data as ServiceSlicePage).sourceVersion, target)}
        />
      )}
      {section === "YAML" && kind === "Secret" && onEdit && (
        <div className="evidence-toolbar yaml-toolbar">
          <button onClick={() => onEdit(fixedEditTarget(row.resource), true)}>Reveal YAML</button>
          <button onClick={() => onEdit(fixedEditTarget(row.resource))}>Edit YAML</button>
          {onApply && <button onClick={() => onApply(fixedEditTarget(row.resource))}>Apply configuration…</button>}
        </div>
      )}
      {section === "YAML" && kind !== "Secret" && !data && onEdit && (
        <div className="evidence-toolbar yaml-toolbar">
          <button onClick={() => onEdit(fixedEditTarget(row.resource))}>Edit YAML</button>
          {onApply && <button onClick={() => onApply(fixedEditTarget(row.resource))}>Apply configuration…</button>}
        </div>
      )}
      {section === "YAML" && kind !== "Secret" && data && (
        <ResourceYaml
          key={JSON.stringify([row.resource.uid, (data as YAMLDocument).fetchedAt])}
          data={data as YAMLDocument}
          containsConfigData={kind === "ConfigMap"}
          onEdit={onEdit && (() => onEdit(fixedEditTarget(row.resource)))}
          onApply={onApply && (() => onApply(fixedEditTarget(row.resource)))}
        />
      )}
      {section === "Events" && data && <ResourceEvents key={row.resource.uid} data={data as EventPage} onNext={c.nextSection} />}
      {section === "Selected Pods" && data && (
        <SelectedPods
          data={data as SelectedPodPage}
          onNext={c.nextSection}
          onSelect={(target) => c.openAvailabilityReference(data as SelectedPodPage, target)}
        />
      )}
      {section === "Revisions" && data && !state?.loading && !state?.error && (
        <ControllerRevisions
          key={JSON.stringify([
            row.resource,
            canReviewOperation,
            c.inventory.sectionEpoch,
            c.inventory.sectionRefresh,
            c.inventory.sectionCursor,
          ])}
          data={data as RolloutHistoryPage}
          resource={row.resource}
          context={context}
          canReview={canReviewOperation}
          onNext={c.nextSection}
          onInspect={(target) =>
            target.kind === "ReplicaSet" ? c.openRelated(target) : c.openAvailabilityReference(data as RolloutHistoryPage, target)
          }
          onConfirm={(token) => onOperation("rollback", token, "")}
        />
      )}
      {section === "Pods" && data && (
        <DeploymentPods
          showOwner={row.resource.kind === "Deployment"}
          data={data as RelatedPage}
          onNext={c.nextSection}
          onSelect={c.openRelated}
        />
      )}
      {section === "ReplicaSets" && data && (
        <ReplicaSetList data={data as RelatedReplicaSetPage} onNext={c.nextSection} onSelect={c.openRelated} />
      )}
      {section === "Jobs" && data && <RelatedJobs data={data as RelatedJobPage} onNext={c.nextSection} onSelect={c.openRelated} />}
      {section === "Logs" && logs}
    </div>
  );
}
