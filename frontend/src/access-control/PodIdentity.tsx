/*
Purpose: Show the captured Pod service account and token automount setting.
Used by: Pod Overview.
Responsibilities: Offer a link only for a named ServiceAccount in the Pod namespace.
Important: No token request or lookup is made to infer mount behavior.
*/
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { accountTarget, automount } from "./model";
import { Field } from "./PolicyRules";

export function PodIdentity({ data, onOpenReference }: { data: ResourceDetail; onOpenReference: (target: ResourceRef) => void }) {
  const { spec } = JSON.parse(data.objectJSON) as { spec?: { serviceAccountName?: string; automountServiceAccountToken?: boolean } };
  const target = accountTarget(data.resource, spec?.serviceAccountName);
  return (
    <section className="evidence-section">
      <h3>Access control</h3>
      <dl className="pod-facts">
        <Field label="Service account">
          {target ? (
            <button className="network-value" onClick={() => onOpenReference(target)}>
              {target.name}
            </button>
          ) : (
            spec?.serviceAccountName || "Not specified"
          )}
        </Field>
        <Field label="Pod automount service account token">{automount(spec?.automountServiceAccountToken)}</Field>
      </dl>
      <p className="muted">These are captured Pod settings. They do not establish whether a token is mounted.</p>
    </section>
  );
}
