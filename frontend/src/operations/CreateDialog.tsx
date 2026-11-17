/*
Purpose: Create one named resource from an in-memory YAML draft.
Used by: The inventory toolbar.
Responsibilities: Keep explicit scope, show the server preview and confirm one token.
Important: Unknown results leave the draft available for deliberate reconciliation.
*/
import { useEffect, useLayoutEffect, useState } from "react";
import type { ConnectionRef } from "../bridge";
import { message } from "../connections/api";
import { CopyTextButton } from "../ui/CopyTextButton";
import { Modal } from "../ui/ReviewDialog";
import { closeCreate, openCreate, prepareCreate, revokeCreate, type CreateDraft, type CreatePreview } from "./api";
import { createSkeleton, canConfirmCreate, type CreateDestination } from "./createState";
import { sameEditConnection } from "./editState";
import { YAMLDiff } from "./YAMLDiff";
import { reviewOwner } from "./reviewState";

export function CreateDialog({
  destination,
  context,
  connection,
  suspended,
  closing,
  replacing,
  onSuspend,
  onCancelClose,
  onClose,
  onConfirm,
}: {
  destination: CreateDestination;
  context: string;
  connection?: ConnectionRef;
  suspended: boolean;
  closing: boolean;
  replacing: boolean;
  onSuspend: () => void;
  onCancelClose: () => void;
  onClose: () => void;
  onConfirm: (token: string) => void;
}) {
  const [text, setText] = useState(createSkeleton);
  const [namespace, setNamespace] = useState(destination.namespace);
  const [clusterScoped, setClusterScoped] = useState(!destination.namespace);
  const [resource, setResource] = useState("");
  const [draft, setDraft] = useState<CreateDraft>();
  const [owner, setOwner] = useState(() => reviewOwner(revokeCreate));
  const [preview, setPreview] = useState<CreatePreview>();
  const [loading, setLoading] = useState(true);
  const [reviewing, setReviewing] = useState(false);
  const [discard, setDiscard] = useState(false);
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const connected = sameEditConnection(destination.connection, connection);
  const selectedNamespace = clusterScoped ? "" : namespace.trim();
  const selectedResource = resource.trim();
  const dirty = text !== createSkeleton || selectedNamespace !== destination.namespace || selectedResource !== "";

  useLayoutEffect(() => {
    let current = true;
    let id = "";
    const request = openCreate(destination.connection, destination.namespace);
    void request.promise.then(
      (value) => {
        if (!current) {
          closeCreate(value.id);
          return;
        }
        id = value.id;
        setDraft(value);
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
      closeCreate(id);
    };
  }, [destination.connection, destination.namespace, attempt]);

  useLayoutEffect(() => owner.abandon, [owner]);
  useLayoutEffect(() => {
    if (connected && !suspended) return;
    owner.abandon();
    setPreview(undefined);
    setReviewing(false);
    if (!connected && draft) closeCreate(draft.id);
  }, [connected, suspended, draft, owner]);

  useEffect(() => {
    if (!preview) return;
    const timer = window.setTimeout(
      () => {
        owner.abandon();
        setPreview(undefined);
        setError("The review expired. Review the create again.");
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
  function review() {
    if (!draft || !connected || (!clusterScoped && !selectedNamespace) || !owner.canPrepare()) return;
    setError("");
    setReviewing(true);
    owner.prepare(
      prepareCreate({ draftID: draft.id, yaml: text, namespace: selectedNamespace, resource: selectedResource }),
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
  function confirm() {
    if (
      !preview ||
      !connection ||
      !connected ||
      !canConfirmCreate(preview, connection, selectedNamespace, selectedResource, Date.now()) ||
      !owner.claim(preview.token)
    )
      return;
    onConfirm(preview.token);
    setPreview(undefined);
    setOwner(() => reviewOwner(revokeCreate));
  }
  function cancel() {
    if (closing) onCancelClose();
    else if (discard) setDiscard(false);
    else if (dirty) setDiscard(true);
    else onClose();
  }
  if (suspended) return null;
  return (
    <Modal title="Create resource" onCancel={cancel}>
      <p className="frozen-target">
        {draft?.cluster ?? context} / {selectedNamespace || "Cluster scope"}
      </p>
      {closing || discard ? (
        <>
          <p>{dirty ? "Discard unsaved YAML changes?" : replacing ? "Open the requested editor?" : "Close this YAML editor?"}</p>
          <div className="scale-actions">
            <button
              onClick={() => {
                setDiscard(false);
                onCancelClose();
              }}
            >
              Keep editing
            </button>
            <button className="danger" onClick={onClose}>
              {dirty ? "Discard changes" : "Close editor"}
            </button>
          </div>
        </>
      ) : (
        <>
          {!connected && <p role="alert">The original connection is unavailable. Copy your YAML or close this draft.</p>}
          {loading && <p role="status">Opening create draft…</p>}
          {reviewing && <p role="status">Running server dry-run…</p>}
          {error && <p role="alert">{error}</p>}
          {preview ? (
            <>
              <p className="frozen-target">
                {[preview.target.group, preview.target.version].filter(Boolean).join("/")} · {preview.target.resource} ·{" "}
                {preview.target.name}
              </p>
              <p role="status">Server dry-run passed.</p>
              <p className="muted">
                The server preview omits simulated identity and generated metadata. Actual defaults and allocations can differ.
              </p>
              <h3>Proposed YAML</h3>
              <pre className="yaml-view edit-diff" tabIndex={0}>
                {preview.proposedYAML}
              </pre>
              <h3>Server preview changes</h3>
              <YAMLDiff before={preview.proposedYAML} after={preview.serverYAML} />
              {!!preview.warnings?.length && (
                <ul>
                  {preview.warnings.map((warning, i) => (
                    <li key={i}>{warning}</li>
                  ))}
                </ul>
              )}
              <div className="scale-actions">
                <button onClick={resetReview}>Back to editor</button>
                <button
                  className="primary"
                  disabled={
                    !connected || !canConfirmCreate(preview, destination.connection, selectedNamespace, selectedResource, Date.now())
                  }
                  onClick={confirm}
                >
                  Confirm create
                </button>
              </div>
            </>
          ) : (
            <>
              <div className="scale-actions">
                <label>
                  Scope{" "}
                  <select
                    value={clusterScoped ? "cluster" : "namespace"}
                    onChange={(e) => {
                      resetReview();
                      setClusterScoped(e.target.value === "cluster");
                    }}
                  >
                    <option value="namespace">Namespace</option>
                    <option value="cluster">Cluster scope</option>
                  </select>
                </label>
                {!clusterScoped && (
                  <label>
                    Namespace{" "}
                    <input
                      value={namespace}
                      spellCheck={false}
                      onChange={(e) => {
                        resetReview();
                        setNamespace(e.target.value);
                      }}
                    />
                  </label>
                )}
              </div>
              <label className="scale-field">
                API resource (only if kind is ambiguous)
                <input
                  value={resource}
                  spellCheck={false}
                  onChange={(e) => {
                    resetReview();
                    setResource(e.target.value);
                  }}
                  placeholder="For example, widgets"
                />
              </label>
              <textarea
                aria-label="New resource YAML"
                className="yaml-view edit-source"
                value={text}
                spellCheck={false}
                autoCorrect="off"
                autoCapitalize="none"
                wrap="off"
                onChange={(e) => {
                  resetReview();
                  setText(e.target.value);
                }}
              />
              <div className="scale-actions">
                <CopyTextButton label="Copy YAML" copiedLabel="YAML copied" text={text} />
                <button onClick={onSuspend}>Keep draft</button>
                {connected && !loading && (!draft || error.includes("capture expired")) && (
                  <button
                    onClick={() => {
                      resetReview();
                      setLoading(true);
                      setDraft(undefined);
                      setAttempt(attempt + 1);
                    }}
                  >
                    Renew draft
                  </button>
                )}
                <button
                  className="primary"
                  disabled={!connected || !draft || reviewing || (!clusterScoped && !selectedNamespace)}
                  onClick={review}
                >
                  Review create
                </button>
              </div>
            </>
          )}
        </>
      )}
    </Modal>
  );
}
