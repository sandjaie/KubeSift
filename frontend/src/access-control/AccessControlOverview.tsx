/*
Purpose: Render captured ServiceAccount and RBAC configuration.
Used by: Resource detail.
Responsibilities: Preserve role and subject scope and offer only explicit reference links.
Important: Captured RBAC configuration does not establish effective access.
*/
import type { ResourceDetail, ResourceRef } from "../workloads/model";
import { EvidencePage } from "../storage/StorageOverview";
import { evidenceTime } from "../workspace/ResourceOverview";
import { BindingSubjects } from "./BindingSubjects";
import { Disclosure, Field, PolicyRules } from "./PolicyRules";
import { automount, roleTarget, type Aggregation, type RoleRef, type Rule, type Subject } from "./model";

type Capture = {
  metadata?: { creationTimestamp?: string };
  automountServiceAccountToken?: boolean;
  secrets?: { name?: string; namespace?: string; uid?: string }[];
  imagePullSecrets?: { name?: string }[];
  rules?: Rule[] | null;
  aggregationRule?: Aggregation;
  roleRef?: RoleRef;
  subjects?: Subject[];
};

export function AccessControlOverview({ data, onOpenReference }: { data: ResourceDetail; onOpenReference: (target: ResourceRef) => void }) {
  const { resource } = data;
  const body = JSON.parse(data.objectJSON) as Capture;
  const binding = resource.kind === "RoleBinding" || resource.kind === "ClusterRoleBinding";
  const role = binding ? roleTarget(resource, body.roleRef) : undefined;
  const roleProblem =
    body.roleRef?.apiGroup !== "rbac.authorization.k8s.io"
      ? "RBAC API group required"
      : body.roleRef.kind !== "ClusterRole" && !(body.roleRef.kind === "Role" && resource.kind === "RoleBinding")
        ? "Role kind is unavailable for this binding scope"
        : "Invalid role name";
  return (
    <div className="evidence-section workload-overview" key={JSON.stringify([resource.uid, data.resourceVersion])}>
      {resource.kind === "ServiceAccount" ? (
        <>
          <p className="muted">
            These settings and references describe configuration. They do not establish token mounting or usable credentials.
          </p>
          <dl className="pod-facts">
            <Field label="Name">{resource.name}</Field>
            <Field label="Namespace">{resource.namespace}</Field>
            <Field label="UID">{resource.uid}</Field>
            <Field label="Created">{evidenceTime(body.metadata?.creationTimestamp)}</Field>
            <Field label="Automount service account token">{automount(body.automountServiceAccountToken)}</Field>
          </dl>
          {(
            [
              ["Image pull references", body.imagePullSecrets],
              ["Legacy Secret references", body.secrets],
            ] as const
          ).map(([label, refs]) => (
            <Disclosure key={label} label={`${label} · ${refs?.length ?? 0} captured`}>
              <EvidencePage
                items={refs ?? []}
                render={(ref, i) => (
                  <p key={i} className="network-value">
                    {ref.name || "Name not reported"}
                  </p>
                )}
              />
            </Disclosure>
          ))}
        </>
      ) : binding ? (
        <>
          <p className="muted">
            {resource.kind === "RoleBinding"
              ? "Referenced rules are bound within this RoleBinding’s namespace. Subject namespace does not change that scope."
              : "This is a cluster-scoped binding. A subject’s namespace identifies its ServiceAccount."}
          </p>
          <dl className="pod-facts">
            <Field label="Binding scope">{resource.namespace || "Cluster"}</Field>
            <Field label="Role API group">{body.roleRef?.apiGroup || "Not specified"}</Field>
            <Field label="Role kind">{body.roleRef?.kind || "Not specified"}</Field>
            <Field label="Role name">
              {role ? (
                <button className="network-value" onClick={() => onOpenReference(role)}>
                  {role.name}
                </button>
              ) : (
                <>
                  {body.roleRef?.name || "Not specified"} · {roleProblem}
                </>
              )}
            </Field>
          </dl>
          <BindingSubjects source={resource} subjects={body.subjects} onOpenReference={onOpenReference} />
        </>
      ) : (
        <PolicyRules
          rules={body.rules}
          aggregationRule={resource.kind === "ClusterRole" ? body.aggregationRule : undefined}
          namespaced={resource.kind === "Role"}
        />
      )}
      <p className="muted">
        Captured {evidenceTime(data.fetchedAt)} · resourceVersion {data.resourceVersion || "Unknown"}
      </p>
    </div>
  );
}
