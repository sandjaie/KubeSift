/*
Purpose: Settle a suspended apply submission without replacing a different draft.
Used by: The editor session and delayed-response tests.
Responsibilities: Bind completion to the submitted editor identity.
Important: Unknown and rejected submissions leave the original YAML available.
*/
import type { EditorDestination, EditorSession } from "./editState";

export type ApplyObservation = { before: string; observed?: string };

export function settleApplyEditor(
  current: EditorSession<EditorDestination> | undefined,
  id: string,
  accepted: boolean,
  observation?: ApplyObservation,
) {
  if (current?.mode !== "apply" || current.id !== id || !current.submitting) return current;
  return accepted && !observation ? undefined : { ...current, submitting: false, observation };
}

export function applyEditorHidden(suspended: boolean, originExists: boolean, inOriginWorkspace: boolean) {
  return suspended || (originExists && !inOriginWorkspace);
}
