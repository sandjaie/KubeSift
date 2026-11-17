/*
Purpose: Show generic object identity and captured text.
Used by: Installed API detail pane.
Responsibilities: Display and copy exact text with bounded DOM and no HTML interpretation.
Important: Unknown fields and number tokens stay in their native capture.
*/
import { CopyTextButton } from "../ui/CopyTextButton";
import { Disclosure, Field } from "../access-control/PolicyRules";
import { evidenceTime } from "../workspace/ResourceOverview";
import type { GenericResourceDetail as Capture } from "./model";

export function GenericResourceDetail({
  capture,
  section,
  onSection,
  onRefresh,
  onEdit,
  onApply,
  canPatch,
  onBack,
  loading,
}: {
  capture: Capture;
  section: "Overview" | "YAML";
  onSection: (section: "Overview" | "YAML") => void;
  onRefresh: () => void;
  onEdit?: () => void;
  onApply?: () => void;
  canPatch: boolean;
  onBack: () => void;
  loading: boolean;
}) {
  const r = capture.resource;
  return (
    <div className="compact-pod-detail">
      <button onClick={onBack}>← Back to inventory</button>
      <div className="pane-heading">
        <h2>{r.name}</h2>
        <button disabled={loading} onClick={onRefresh}>
          Refresh object
        </button>
      </div>
      <p className="detail-identity">
        {r.namespace || "Cluster-scoped"} / {r.kind} / {r.group || "core"}/{r.version}
      </p>
      <div className="detail-tabs" role="tablist">
        {(["Overview", "YAML"] as const).map((tab) => (
          <button key={tab} role="tab" aria-selected={section === tab} onClick={() => onSection(tab)}>
            {tab}
          </button>
        ))}
      </div>
      {section === "Overview" ? (
        <div className="evidence-section">
          <dl className="pod-facts">
            <Field label="Resource">{r.resource}</Field>
            <Field label="UID">{r.uid}</Field>
            <Field label="Resource version">{capture.resourceVersion || "Unknown"}</Field>
          </dl>
          <Disclosure label="Captured object JSON">
            <p className="muted yaml-note">Installed API objects can contain sensitive values. Check before sharing.</p>
            <textarea aria-label="Read-only captured JSON" className="yaml-view" readOnly spellCheck={false} value={capture.objectJSON} />
          </Disclosure>
        </div>
      ) : (
        <div className="evidence-section yaml-evidence">
          <div className="evidence-toolbar yaml-toolbar">
            <button
              onClick={onEdit}
              disabled={!canPatch || loading || !onEdit}
              title={canPatch ? undefined : "This API does not advertise Patch"}
            >
              Edit YAML
            </button>
            <button onClick={onApply} disabled={!canPatch || loading || !onApply}>
              Apply configuration…
            </button>
            {!canPatch && <span className="muted">This API does not advertise Patch.</span>}
            <CopyTextButton
              label="Copy displayed YAML"
              copiedLabel="Displayed YAML copied"
              key={JSON.stringify([r.uid, capture.fetchedAt])}
              text={capture.yaml}
            />
            <span className="muted">Installed API objects can contain sensitive values. Check before sharing.</span>
          </div>
          <textarea aria-label="Read-only server YAML" className="yaml-view" readOnly spellCheck={false} wrap="off" value={capture.yaml} />
        </div>
      )}
      <p className="muted">Captured {evidenceTime(capture.fetchedAt)}</p>
    </div>
  );
}
