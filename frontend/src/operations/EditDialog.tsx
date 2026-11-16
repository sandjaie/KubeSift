/*
Purpose: Edit one captured resource and review the server dry-run.
Used by: Fixed, discovered and Secret YAML views.
Responsibilities: Retain drafts in memory, discard late reviews and confirm one token.
Important: YAML is visible only within the deliberately opened editor.
*/
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ConnectionRef } from "../bridge";
import { message } from "../connections/api";
import { CopyTextButton } from "../ui/CopyTextButton";
import { Modal } from "../ui/ReviewDialog";
import { closeEdit, openEdit, prepareEdit, revokeEdit, type EditDraft, type EditPreview, type EditTarget } from "./api";
import { canConfirmEdit, editDiff, sameEditConnection } from "./editState";
import { reviewOwner } from "./reviewState";

function Diff({ before, after }: { before: string; after: string }) {
  return (
    <pre className="yaml-view edit-diff" tabIndex={0}>
      {editDiff(before, after)}
    </pre>
  );
}

export function EditDialog({
  target,
  context,
  connection,
  suspended,
  reveal,
  closing,
  replacing,
  onSuspend,
  onCancelClose,
  onClose,
  onConfirm,
}: {
  target: EditTarget;
  context: string;
  connection?: ConnectionRef;
  suspended: boolean;
  reveal: boolean;
  closing: boolean;
  replacing?: boolean;
  onSuspend: () => void;
  onCancelClose: () => void;
  onClose: () => void;
  onConfirm: (token: string, draftID: string) => void;
}) {
  const [requestTarget, setRequestTarget] = useState({ target });
  const [draft, setDraft] = useState<EditDraft>();
  const [text, setText] = useState("");
  const [editing, setEditing] = useState(!reveal);
  const [preview, setPreview] = useState<EditPreview>();
  const [loading, setLoading] = useState(true);
  const [reviewing, setReviewing] = useState(false);
  const [error, setError] = useState("");
  const [discard, setDiscard] = useState<"close" | "refresh">();
  const [owner] = useState(() => reviewOwner(revokeEdit));
  const claimed = useRef(false);
  const [invalidatedID, setInvalidatedID] = useState("");
  const connectedToDraft = sameEditConnection((draft?.target ?? requestTarget.target).connection, connection);
  const available = connectedToDraft && (!draft || invalidatedID !== draft.id);
  const dirty = !!draft && text !== draft.yaml;

  useLayoutEffect(() => {
    let current = true;
    let id = "";
    const request = openEdit(requestTarget.target);
    void request.promise.then(
      (value) => {
        if (!current) {
          closeEdit(value.id);
          return;
        }
        id = value.id;
        setDraft(value);
        setText(value.yaml);
        setLoading(false);
      },
      (failure) => {
        if (!current) return;
        setError(message(failure));
        setLoading(false);
      },
    );
    return () => {
      current = false;
      request.cancel();
      if (!claimed.current) closeEdit(id);
    };
  }, [requestTarget]);

  useLayoutEffect(() => {
    if (!suspended && connectedToDraft) return;
    owner.abandon();
    setPreview(undefined);
    setReviewing(false);
    if (!connectedToDraft && draft) {
      setInvalidatedID(draft.id);
      closeEdit(draft.id);
    }
  }, [suspended, connectedToDraft, draft, owner]);

  useLayoutEffect(() => owner.abandon, [owner]);
  useEffect(() => {
    if (!preview) return;
    const timer = window.setTimeout(
      () => {
        owner.abandon();
        setPreview(undefined);
        setError("The review expired. Review the changes again.");
      },
      Math.max(0, Date.parse(preview.expiresAt) - Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [preview, owner]);

  function resetReview() {
    owner.abandon();
    setPreview(undefined);
    setReviewing(false);
    setError("");
  }
  function refresh() {
    if (!connection) return;
    resetReview();
    setDiscard(undefined);
    setLoading(true);
    setDraft(undefined);
    setText("");
    setRequestTarget({ target: { ...target, connection } });
  }
  function review() {
    if (!draft || !available || !owner.canPrepare()) return;
    setError("");
    setReviewing(true);
    owner.prepare(
      prepareEdit({ draftID: draft.id, yaml: text }),
      (value) => {
        setPreview(value);
        setReviewing(false);
      },
      (failure) => {
        setError(message(failure));
        setReviewing(false);
      },
    );
  }
  function submit() {
    if (!draft || !preview || !available || !canConfirmEdit(preview, draft.target, Date.now()) || !owner.claim(preview.token)) return;
    claimed.current = true;
    onConfirm(preview.token, draft.id);
  }
  function cancel() {
    if (closing) onCancelClose();
    else if (discard) setDiscard(undefined);
    else if (dirty) setDiscard("close");
    else onClose();
  }
  if (suspended) return null;
  const targetIdentity = draft?.target ?? target;
  const secret = !targetIdentity.group && targetIdentity.kind === "Secret";
  const confirmingDiscard = discard || closing;
  return (
    <Modal title={editing ? "Edit YAML" : "Reveal YAML"} onCancel={cancel}>
      <p className="frozen-target">
        {draft?.cluster ?? context} / {targetIdentity.namespace || "Cluster-scoped"} / {targetIdentity.kind} / {targetIdentity.name}
      </p>
      {!!draft && (
        <p className="muted">
          UID {targetIdentity.uid} · version {draft.resourceVersion}
        </p>
      )}
      {confirmingDiscard ? (
        <>
          <p>{dirty ? "Discard unsaved YAML changes?" : replacing ? "Open the requested resource?" : "Close this YAML editor?"}</p>
          <div className="scale-actions">
            <button
              onClick={() => {
                setDiscard(undefined);
                onCancelClose();
              }}
            >
              Keep editing
            </button>
            <button className="danger" onClick={discard === "refresh" && !closing ? refresh : onClose}>
              {dirty ? "Discard changes" : replacing ? "Open resource" : "Close editor"}
            </button>
          </div>
        </>
      ) : (
        <>
          {loading && <p role="status">Reading the resource from Kubernetes…</p>}
          {!available && (
            <p role="alert">
              The original capture is disconnected or its connection changed. Refresh from Kubernetes to start a new review.
            </p>
          )}
          {error && <p role="alert">{error}</p>}
          {draft && !preview && !loading && (
            <>
              {secret && <p className="muted">Secret data is base64, not encryption. Use stringData for text values.</p>}
              <textarea
                aria-label={editing ? "Resource YAML" : "Revealed YAML"}
                className="yaml-view edit-source"
                value={text}
                readOnly={!editing}
                spellCheck={false}
                autoCorrect="off"
                autoCapitalize="none"
                wrap="off"
                onChange={(event) => {
                  resetReview();
                  setText(event.target.value);
                }}
              />
            </>
          )}
          {reviewing && <p role="status">Running server dry-run…</p>}
          {preview && (
            <>
              <p role="status">Server dry-run passed.</p>
              <Diff before={preview.beforeYAML} after={preview.proposedYAML} />
              {preview.serverYAML && (
                <details>
                  <summary>Additional server changes</summary>
                  <Diff before={preview.proposedYAML} after={preview.serverYAML} />
                </details>
              )}
              {!!preview.warnings?.length && (
                <ul>
                  {preview.warnings.map((warning, i) => (
                    <li key={i}>{warning}</li>
                  ))}
                </ul>
              )}
            </>
          )}
          <div className="scale-actions">
            {draft && !preview && !loading && <CopyTextButton label="Copy YAML" copiedLabel="YAML copied" text={text} />}
            {draft && editing && <button onClick={onSuspend}>Keep draft</button>}
            {preview && <button onClick={resetReview}>Back to editor</button>}
            {!loading && !preview && (
              <button disabled={!connection} onClick={() => (dirty ? setDiscard("refresh") : refresh())}>
                Refresh from Kubernetes
              </button>
            )}
            {draft && !editing && <button onClick={() => setEditing(true)}>Edit YAML</button>}
            {draft && editing && !preview && (
              <button className="primary" disabled={!dirty || !available || reviewing || loading} onClick={review}>
                Review changes
              </button>
            )}
            {draft && preview && (
              <button className="primary" disabled={!available || !canConfirmEdit(preview, draft.target, Date.now())} onClick={submit}>
                Confirm
              </button>
            )}
          </div>
        </>
      )}
    </Modal>
  );
}
