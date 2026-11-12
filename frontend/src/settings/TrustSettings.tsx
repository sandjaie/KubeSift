/*
Purpose: Review and revoke native credential-helper approvals in Settings.
Used by: SettingsPage and connection review navigation.
Responsibilities: Show redacted helper configuration and leave connection execution explicit.
Important: Approval does not run a helper; revocation closes all live connections.
*/
import { useEffect, useState } from "react";
import { api, message, type CatalogEntry, type PreviewRecord, type TrustedHelper } from "../connections/api";
import type { ConnectionsController } from "../connections/useConnections";
import { tabKey } from "../connections/state";
import { Modal } from "../ui/ReviewDialog";
export function TrustSettings({
  controller,
  target,
  onConnect,
}: {
  controller: ConnectionsController;
  target: CatalogEntry | null;
  onConnect: (entry: CatalogEntry) => void;
}) {
  const entries = controller.state.sources.flatMap((s) => s.contexts ?? []);
  const [selected, setSelected] = useState(target ? tabKey(target) : "");
  const [review, setReview] = useState<{ entry: CatalogEntry; preview: PreviewRecord } | null>(null);
  const [trusted, setTrusted] = useState<TrustedHelper[]>([]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [revoke, setRevoke] = useState<TrustedHelper | null>(null);
  const [busy, setBusy] = useState(false);
  const entry = entries.find((e) => tabKey(e) === selected);
  async function load() {
    try {
      setTrusted((await api.TrustedHelpers()) ?? []);
    } catch (e) {
      setError(message(e));
    }
  }
  useEffect(() => {
    void load();
  }, []);
  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
      await load();
      await controller.refreshSources();
      await controller.poll();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <h2>Credential-helper trust</h2>
      <p>
        Sign in externally in your terminal, then Retry. KubeSift only runs configured noninteractive credential helpers after approval.
        Approved executables are not sandboxed.
      </p>
      {error && (
        <p role="alert" className="settings-error">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      <label>
        Context to review
        <select
          value={selected}
          onChange={(e) => {
            setSelected(e.target.value);
            setReview(null);
            setNotice("");
          }}
        >
          <option value="">Choose a context</option>
          {entries.map((e) => (
            <option key={tabKey(e)} value={tabKey(e)}>
              {e.context} — {e.sourceDisplay}
            </option>
          ))}
        </select>
      </label>
      <div className="button-row">
        <button
          disabled={busy || !entry}
          onClick={() =>
            entry &&
            void run(async () => {
              setReview({ entry, preview: await api.ReviewHelper(entry.sourceId, entry.snapshotId, entry.context) });
            })
          }
        >
          Review helper
        </button>
        <button disabled={busy || !entry} onClick={() => entry && onConnect(entry)}>
          Connect
        </button>
      </div>
      {review && (
        <div className="helper-review">
          <h3>Review executable</h3>
          <p>{review.preview.executable}</p>
          <p>Context: {review.preview.context}</p>
          <p className="source-path">Source: {review.preview.source}</p>
          <p>Arguments: {review.preview.args?.join(" ") || "None"}</p>
          <p>Environment: {review.preview.env?.join(", ") || "None"}</p>
          <p>Only approve a credential retrieval helper you trust. This is not a login command.</p>
          <button
            disabled={busy}
            onClick={() =>
              void run(async () => {
                await api.ApproveHelper(review.entry.sourceId, review.entry.snapshotId, review.entry.context, review.preview.id);
                setReview(null);
                setNotice("Approved. Select Connect to start; approval has not executed the helper.");
              })
            }
          >
            Approve helper
          </button>
        </div>
      )}
      <h3>Remembered approvals</h3>
      {trusted.length === 0 ? (
        <p>No remembered approvals.</p>
      ) : (
        trusted.map((t) => (
          <div className="source-row" key={t.fingerprint}>
            <span>{t.label}</span>
            <button disabled={busy} onClick={() => setRevoke(t)}>
              Revoke
            </button>
          </div>
        ))
      )}
      {revoke && (
        <Modal title="Revoke helper and close all connections?" onCancel={() => setRevoke(null)}>
          <p>
            This closes all live connections and owned sessions, including contexts using other credentials. Reconnection requires an
            explicit action.
          </p>
          <button
            disabled={busy}
            onClick={() => {
              const fp = revoke.fingerprint;
              setRevoke(null);
              void run(async () => {
                await api.RevokeHelper(fp);
                setNotice("Approval revoked. Connections are closed.");
              });
            }}
          >
            Revoke and close all
          </button>
        </Modal>
      )}
    </section>
  );
}
