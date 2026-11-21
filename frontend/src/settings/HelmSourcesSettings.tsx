/*
Purpose: Configure Helm repositories and native session authentication in existing Settings.
Used by: SettingsPage.
Responsibilities: Edit nonsecret metadata, select TLS files and clear submitted credential inputs.
Important: Passwords are never persisted, refilled or returned by the native service.
*/
import { useEffect, useState } from "react";
import type { HelmSource } from "../../bindings/internal/preferences/models";
import { message } from "../connections/api";
import { helmManagementAPI as api } from "../helm/managementApi";

const emptySource = (): HelmSource => ({
  id: crypto.randomUUID(),
  name: "",
  kind: "https",
  url: "",
  caFile: "",
  certFile: "",
  keyFile: "",
});

export function HelmSourcesSettings() {
  const [sources, setSources] = useState<HelmSource[]>([]);
  const [draft, setDraft] = useState(emptySource);
  const [credentialSource, setCredentialSource] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    let active = true;
    void api.ListHelmSources().then(
      (items) => {
        if (active) setSources(items ?? []);
      },
      (reason: unknown) => {
        if (active) setError(message(reason));
      },
    );
    return () => {
      active = false;
    };
  }, []);

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setSources((await api.ListHelmSources()) ?? []);
    } catch (reason) {
      setError(message(reason));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section>
      <h2>Helm sources</h2>
      <p className="muted">
        Save HTTPS repositories or OCI registry paths. Editing or removing a source clears its credentials and unfinished drafts.
        Already-running actions keep their pinned chart.
      </p>
      {error && (
        <p role="alert" className="settings-error">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {sources.map((source) => (
        <div className="source-row" key={source.id}>
          <strong>{source.name}</strong>
          <p className="source-path">{source.url}</p>
          <div className="button-row">
            <button disabled={busy} onClick={() => setDraft({ ...source })}>
              Edit
            </button>
            <button
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  await api.ForgetHelmCredentials(source.id);
                  setPassword("");
                  setNotice(`Credentials forgotten for ${source.name}.`);
                })
              }
            >
              Forget credentials
            </button>
            <button
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  await api.DeleteHelmSource(source.id);
                  if (draft.id === source.id) setDraft(emptySource());
                  if (credentialSource === source.id) {
                    setCredentialSource("");
                    setUsername("");
                    setPassword("");
                  }
                })
              }
            >
              Remove
            </button>
          </div>
        </div>
      ))}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void run(async () => {
            await api.SaveHelmSource(draft);
            setCredentialSource(draft.id);
            setDraft(emptySource());
            setNotice("Helm source saved. Credentials can be supplied for this session below.");
          });
        }}
      >
        <fieldset disabled={busy} className="helm-source-fields">
          <legend>{sources.some((source) => source.id === draft.id) ? "Edit source" : "Add source"}</legend>
          <label>
            Source name
            <input value={draft.name} maxLength={128} required onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
          </label>
          <label>
            Source type
            <select value={draft.kind} onChange={(event) => setDraft({ ...draft, kind: event.target.value })}>
              <option value="https">HTTPS repository</option>
              <option value="oci">OCI registry/path</option>
            </select>
          </label>
          <label>
            Source URL
            <input
              value={draft.url}
              placeholder={draft.kind === "oci" ? "oci://registry.example.com/charts" : "https://charts.example.com"}
              autoComplete="off"
              spellCheck={false}
              required
              maxLength={4096}
              onChange={(event) => setDraft({ ...draft, url: event.target.value })}
            />
          </label>
          {(
            [
              ["ca", "CA certificate", "caFile"],
              ["cert", "Client certificate", "certFile"],
              ["key", "Client key", "keyFile"],
            ] as const
          ).map(([kind, label, field]) => (
            <div key={kind} className="button-row">
              <button
                type="button"
                onClick={() =>
                  void run(async () => {
                    const path = await api.PickHelmTLSFile(kind);
                    setDraft((current) => ({ ...current, [field]: path }));
                  })
                }
              >
                Select {label.toLowerCase()}
              </button>
              <span className="source-path">{draft[field] || "Not selected"}</span>
              {draft[field] && (
                <button type="button" onClick={() => setDraft({ ...draft, [field]: "" })}>
                  Clear {label.toLowerCase()}
                </button>
              )}
            </div>
          ))}
          <div className="button-row">
            <button type="submit">Save Helm source</button>
            <button type="button" onClick={() => setDraft(emptySource())}>
              New source
            </button>
          </div>
        </fieldset>
      </form>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          const credentials = { username, password };
          setUsername("");
          setPassword("");
          void run(async () => {
            await api.SetHelmCredentials(credentialSource, credentials);
            setNotice("Credentials are available in native memory for this session only.");
          });
        }}
      >
        <fieldset disabled={busy} className="helm-source-fields">
          <legend>Session credentials</legend>
          <p className="muted">Obtain registry tokens externally. Credentials are cleared on Forget or app exit; they are never saved.</p>
          <label>
            Authenticate source
            <select
              value={credentialSource}
              required
              onChange={(event) => {
                setCredentialSource(event.target.value);
                setUsername("");
                setPassword("");
              }}
            >
              <option value="">Choose a saved source</option>
              {sources.map((source) => (
                <option key={source.id} value={source.id}>
                  {source.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            Username
            <input value={username} autoComplete="off" maxLength={4096} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label>
            Password or token
            <input
              type="password"
              value={password}
              autoComplete="new-password"
              required
              maxLength={65536}
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>
          <button disabled={!credentialSource || !password} type="submit">
            Use for this session
          </button>
        </fieldset>
      </form>
    </section>
  );
}
