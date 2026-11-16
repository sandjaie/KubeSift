/*
Purpose: Bound display and confirmation of a YAML edit preview.
Used by: The common editor and its state tests.
Responsibilities: Compare canonical targets and produce a small line diff.
Important: The native service owns the patch and verifies the target.
*/
import type { ConnectionRef } from "../bridge";
import type { ResourceRef } from "../workloads/model";
import type { EditPreview, EditTarget } from "./api";

export function sameEditConnection(a: ConnectionRef, b?: ConnectionRef) {
  return a.id === b?.id && a.generation === b.generation;
}

export function sameEditTarget(a: EditTarget, b: EditTarget) {
  return (
    sameEditConnection(a.connection, b.connection) &&
    a.group === b.group &&
    a.version === b.version &&
    a.resource === b.resource &&
    a.kind === b.kind &&
    a.namespace === b.namespace &&
    a.name === b.name &&
    a.uid === b.uid
  );
}

export function fixedEditTarget(ref: ResourceRef): EditTarget {
  const [group, version] = ref.apiVersion.includes("/") ? ref.apiVersion.split("/") : ["", ref.apiVersion];
  return {
    connection: ref.connection,
    group,
    version,
    resource: "",
    kind: ref.kind,
    namespace: ref.namespace,
    name: ref.name,
    uid: ref.uid,
  };
}

export function canConfirmEdit(preview: Pick<EditPreview, "token" | "expiresAt" | "target">, target: EditTarget, now: number) {
  return !!preview.token && now < Date.parse(preview.expiresAt) && sameEditTarget(preview.target, target);
}

export function editDiff(before: string, after: string): string | null {
  if (before.length + after.length > 65536) return null;
  const a = before.split("\n");
  const b = after.split("\n");
  if (a.length + b.length > 800) return null;
  const common = Array.from({ length: a.length + 1 }, () => new Uint16Array(b.length + 1));
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      common[i][j] = a[i] === b[j] ? common[i + 1][j + 1] + 1 : Math.max(common[i + 1][j], common[i][j + 1]);
    }
  }
  const lines: string[] = [];
  let i = 0;
  let j = 0;
  while (i < a.length || j < b.length) {
    if (i < a.length && j < b.length && a[i] === b[j]) {
      lines.push("  " + a[i++]);
      j++;
    } else if (i < a.length && (j === b.length || common[i + 1][j] >= common[i][j + 1])) {
      lines.push("- " + a[i++]);
    } else {
      lines.push("+ " + b[j++]);
    }
  }
  return lines.join("\n");
}
