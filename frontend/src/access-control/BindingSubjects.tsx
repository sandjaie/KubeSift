/*
Purpose: Display separately captured binding subjects.
Used by: RoleBinding and ClusterRoleBinding Overview.
Responsibilities: Explain lookup namespace and render external subjects as literal text.
Important: Subject namespace is independent of binding scope.
*/
import type { ResourceRef } from "../workloads/model";
import { EvidencePage } from "../storage/StorageOverview";
import { subjectTarget, type Subject } from "./model";

export function BindingSubjects({
  source,
  subjects = [],
  onOpenReference,
}: {
  source: ResourceRef;
  subjects?: Subject[];
  onOpenReference: (target: ResourceRef) => void;
}) {
  return (
    <>
      <h3>Subjects · {subjects.length} captured</h3>
      {!subjects.length ? (
        <p>No subjects captured.</p>
      ) : (
        <EvidencePage
          items={subjects}
          wrap={(rows) => (
            <div className="evidence-table-wrap">
              <table className="evidence-table">
                <thead>
                  <tr>
                    {["Kind", "Name", "API group", "Captured namespace", "Resolution namespace"].map((label) => (
                      <th key={label}>{label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>{rows}</tbody>
              </table>
            </div>
          )}
          render={(subject, index) => {
            const target = subjectTarget(source, subject);
            const reason =
              subject.kind === "User" || subject.kind === "Group"
                ? "External identity; no lookup"
                : subject.kind !== "ServiceAccount"
                  ? "Unsupported subject kind"
                  : subject.apiGroup
                    ? "ServiceAccount API group must be empty"
                    : source.kind === "ClusterRoleBinding" && !subject.namespace
                      ? "Explicit namespace required"
                      : "Invalid ServiceAccount name or namespace";
            return (
              <tr key={index}>
                <td className="network-value">{subject.kind || "Not specified"}</td>
                <td className="network-value">
                  {target ? (
                    <button className="network-value" onClick={() => onOpenReference(target)}>
                      {subject.name}
                    </button>
                  ) : (
                    subject.name || "Not specified"
                  )}
                </td>
                <td className="network-value">
                  {subject.apiGroup || (subject.kind === "ServiceAccount" ? "Core API group" : "Not specified")}
                </td>
                <td className="network-value">{subject.namespace || "Not specified"}</td>
                <td className="network-value">
                  {target ? (subject.namespace ? target.namespace : `${target.namespace} · Binding namespace`) : reason}
                </td>
              </tr>
            );
          }}
        />
      )}
    </>
  );
}
