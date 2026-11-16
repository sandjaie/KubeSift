/*
Purpose: Review the Pod template diff for one retained workload revision.
Used by: The workload Revisions detail section.
Responsibilities: Run dry-run, show strategy notices and confirm a single token.
Important: The current workload and historical source are fixed for the life of the dialog.
*/
import { useEffect, useLayoutEffect, useState } from "react";
import { message } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import { prepareRollback, revokeRollback, type RollbackPreview, type RollbackRequest } from "./api";
import { editDiff } from "./editState";
import { reviewOwner } from "./reviewState";
import { canConfirmRollback } from "./rollbackState";

function Diff({ before, after }: { before: string; after: string }) {
  return (
    <pre className="yaml-view edit-diff" tabIndex={0}>
      {editDiff(before, after)}
    </pre>
  );
}

export function RollbackDialog({
  request,
  context,
  onClose,
  onConfirm,
}: {
  request: RollbackRequest;
  context: string;
  onClose: () => void;
  onConfirm: (token: string) => void;
}) {
  const [owner] = useState(() => reviewOwner(revokeRollback));
  const [preview, setPreview] = useState<RollbackPreview>();
  const [error, setError] = useState("");
  useLayoutEffect(() => {
    owner.prepare(prepareRollback(request), setPreview, (failure) => setError(message(failure)));
    return owner.abandon;
  }, [owner, request]);

  useEffect(() => {
    if (!preview) return;
    const timer = window.setTimeout(
      () => {
        owner.abandon();
        setPreview(undefined);
        setError("The rollback review expired. Select the revision and review again.");
      },
      Math.max(0, Date.parse(preview.expiresAt) - Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [owner, preview]);

  const { resource, source, revision } = request;
  function confirm() {
    if (preview && canConfirmRollback(preview, request, Date.now()) && owner.claim(preview.token)) onConfirm(preview.token);
  }
  return (
    <Modal title="Review workload rollback" onCancel={onClose}>
      <p className="frozen-target">
        {preview?.cluster ?? context} / {resource.namespace} / {resource.kind} / {resource.name}
      </p>
      <p>
        Revision {revision} · {source.kind} {source.name}
      </p>
      <p className="muted">Restores the Pod template. The workload's replica count, storage settings and update strategy stay in place.</p>
      {!preview && !error && <p role="status">Reading the selected revision and running server dry-run…</p>}
      {error && <p role="alert">{error}</p>}
      {preview && (
        <>
          <p role="status">Server dry-run passed.</p>
          {[...(preview.notices ?? []), ...(preview.warnings ?? [])].map((notice, i) => (
            <p className="muted" key={i}>
              {notice}
            </p>
          ))}
          <Diff before={preview.beforeYAML} after={preview.proposedYAML} />
          {preview.serverYAML && (
            <details>
              <summary>Additional server changes</summary>
              <Diff before={preview.proposedYAML} after={preview.serverYAML} />
            </details>
          )}
          <div className="scale-actions">
            <button className="primary" disabled={!canConfirmRollback(preview, request, Date.now())} onClick={confirm}>
              Confirm
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}
