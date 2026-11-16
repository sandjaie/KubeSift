/*
Purpose: Bound display and confirmation of a YAML edit preview.
Used by: The common editor and its state tests.
Responsibilities: Compare canonical targets and produce a small line diff.
Important: The native service owns the patch and verifies the target.
*/
import type { ConnectionRef } from "../bridge";
import type { ResourceRef } from "../workloads/model";
import type { EditPreview, EditTarget, OperationResult } from "./api";

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

export type EditDestination = { tab: string; target: EditTarget; reveal: boolean };
export type EditorSession = EditDestination & { suspended: boolean; closing: boolean; replacement?: EditDestination };

export function requestEditor(current: EditorSession | undefined, next: EditDestination): EditorSession {
  if (!current) return { ...next, suspended: false, closing: false };
  if (current.tab === next.tab && sameEditTarget(current.target, next.target))
    return { ...current, suspended: false, closing: false, replacement: undefined };
  return { ...current, replacement: next };
}

export function savedEditAfterCapture(results: OperationResult[], target: EditTarget, fetchedAt: string) {
  const captured = Date.parse(fetchedAt);
  return results.find(
    (row) =>
      row.state === "accepted" && row.edit && row.finishedAt && Date.parse(row.finishedAt) >= captured && sameEditTarget(row.edit, target),
  )?.operationID;
}

export function editDiff(before: string, after: string): string {
  const a = before.split("\n");
  const b = after.split("\n");
  if (before.length + after.length > 65536 || a.length + b.length > 800) return editChangeSpan(a, b);
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

function editChangeSpan(before: string[], after: string[]) {
  let start = 0;
  let previousEnd = before.length;
  let proposedEnd = after.length;
  while (start < previousEnd && start < proposedEnd && before[start] === after[start]) start++;
  while (previousEnd > start && proposedEnd > start && before[previousEnd - 1] === after[proposedEnd - 1]) {
    previousEnd--;
    proposedEnd--;
  }
  const lines = [`@@ Line ${start + 1} · ${previousEnd - start} original lines, ${proposedEnd - start} proposed lines @@`];
  for (let index = start; index < previousEnd; index++) lines.push("- " + before[index]);
  for (let index = start; index < proposedEnd; index++) lines.push("+ " + after[index]);
  return lines.join("\n");
}
