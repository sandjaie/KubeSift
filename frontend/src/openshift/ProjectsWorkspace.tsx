/*
Purpose: Browse visible Projects and inspect a single captured Project.
Used by: The OpenShift workspace destination.
Responsibilities: Present independent quota and access evidence and deliberate lifecycle reviews.
Important: Selecting a row never changes the working namespace.
*/
import { useState, useEffect, useRef, type ComponentProps } from "react";
import type { Project, ProjectCapabilities, ProjectQuotaPage, ProjectAccess } from "../../bindings/internal/kube/models";
import type { ProjectRequestPreview, DeletePreview } from "../../bindings/internal/operation/models";
import { api, message, type ConnectionRef } from "../connections/api";
import { Modal } from "../ui/ReviewDialog";
import { ResourceSplit } from "../workspace/ResourceSplit";
import type { OperationAction } from "../operations/api";
type Submit = (action: OperationAction, token: string, ack: string) => void;
type Mode = "request-project" | "delete";
export function ProjectsWorkspace({
  connection,
  caps,
  items,
  partial,
  error,
  refresh,
  useProject,
  submit,
  split,
}: {
  connection: ConnectionRef;
  caps: ProjectCapabilities;
  items: Project[];
  partial: boolean;
  error: string;
  refresh: () => void;
  useProject: (p: Project, current: () => boolean, onLost: (error: string) => void) => void;
  submit: Submit;
  split: Omit<ComponentProps<typeof ResourceSplit>, "children">;
}) {
  const [query, setQuery] = useState("");
  const [p, select] = useState<Project>();
  const [problem, fail] = useState("");
  const [mode, open] = useState<Mode>();
  const epoch = useRef(0);
  useEffect(
    () => () => {
      epoch.current++;
    },
    [],
  );
  const captureEpoch = epoch.current;
  function loseCapture(error: string) {
    if (captureEpoch !== epoch.current) return;
    epoch.current++;
    open(undefined);
    select(undefined);
    fail(error);
  }
  async function inspect(name: string, uid = "") {
    const n = ++epoch.current;
    open(undefined);
    select(undefined);
    fail("");
    try {
      const next = await api.ReadProject(crypto.randomUUID(), connection, name, uid);
      if (n === epoch.current) select(next);
    } catch (e) {
      if (n === epoch.current) fail(message(e));
    }
  }
  return (
    <ResourceSplit {...split}>
      <section className="list-pane">
        <div className="pane-heading">
          <h2>Projects</h2>
          <button
            onClick={() => {
              epoch.current++;
              select(undefined);
              refresh();
            }}
          >
            Refresh
          </button>
          {caps.request && <button onClick={() => open("request-project")}>Request project</button>}
        </div>
        {caps.get && (
          <form
            className="pane-heading"
            onSubmit={(e) => {
              e.preventDefault();
              void inspect(query);
            }}
          >
            <label>
              Project name <input value={query} onChange={(e) => setQuery(e.target.value)} />
            </label>
            <button disabled={!query}>Look up</button>
          </form>
        )}
        {error && <p role="alert">{error}</p>}
        {partial && <p>Project list incomplete.</p>}
        {!partial && !items.length && <p>No visible Projects.</p>}
        <table>
          <thead>
            <tr>
              <th>Name</th>
              <th>Display name</th>
              <th>Phase</th>
            </tr>
          </thead>
          <tbody>
            {items.map((row) => (
              <tr key={row.resource.uid}>
                <td>
                  <button onClick={() => (caps.get ? void inspect(row.resource.name, row.resource.uid) : select(row))}>
                    {row.resource.name}
                  </button>
                </td>
                <td>{row.displayName}</td>
                <td>{row.phase}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      <section className="detail-pane">
        {problem && <p role="alert">{problem}</p>}
        {p && (
          <>
            <h2>{p.resource.name}</h2>
            <p>{p.displayName}</p>
            <p>{p.description}</p>
            {p.truncated && <p>Metadata truncated</p>}
            <dl>
              <dt>Phase</dt>
              <dd>{p.phase}</dd>
              <dt>UID</dt>
              <dd>{p.resource.uid}</dd>
              <dt>Resource version</dt>
              <dd>{p.version}</dd>
              <dt>Created</dt>
              <dd>{p.createdAt || "Unknown"}</dd>
              {p.deletingAt && (
                <>
                  <dt>Deleting since</dt>
                  <dd>{p.deletingAt}</dd>
                </>
              )}
            </dl>
            <button
              disabled={!caps.get || p.phase !== "Active" || !!p.deletingAt}
              onClick={() => {
                const n = epoch.current;
                useProject(p, () => n === epoch.current, loseCapture);
              }}
            >
              Use project
            </button>
            {caps.delete && caps.get && (
              <button disabled={!!p.deletingAt || p.phase === "Terminating"} onClick={() => open("delete")}>
                Delete project
              </button>
            )}
            <Evidence key={p.resource.uid} project={p} onLost={loseCapture} />
          </>
        )}
        {mode && (!(mode === "delete") || p) && (
          <Action
            key={mode + ":" + (p?.resource.uid ?? "")}
            mode={mode}
            connection={connection}
            project={p}
            close={() => open(undefined)}
            submit={submit}
          />
        )}
      </section>
    </ResourceSplit>
  );
}
function Evidence({ project, onLost }: { project: Project; onLost: (error: string) => void }) {
  const [quotas, setQ] = useState<ProjectQuotaPage>();
  const [access, setA] = useState<ProjectAccess>();
  const [qe, qfail] = useState("");
  const [ae, afail] = useState("");
  async function quota(cursor: string) {
    setQ(undefined);
    qfail("");
    try {
      setQ(await api.ProjectQuotas(crypto.randomUUID(), project.resource, cursor));
    } catch (e) {
      if (message(e).includes("project_unavailable")) onLost(message(e));
      else qfail(message(e));
    }
  }
  async function check() {
    setA(undefined);
    afail("");
    try {
      setA(await api.MyProjectAccess(crypto.randomUUID(), project.resource));
    } catch (e) {
      if (message(e).includes("project_unavailable")) onLost(message(e));
      else afail(message(e));
    }
  }
  return (
    <>
      <h3>Namespace resource quotas</h3>
      <button onClick={() => void quota("")}>Read quotas</button>
      {qe && <p role="alert">{qe}</p>}
      {quotas && (
        <>
          {!quotas.items?.length && <p>No visible namespace quotas.</p>}
          {quotas.items?.map((q) => (
            <section key={q.name}>
              <strong>{q.name}</strong>
              {Object.entries(q.hard ?? {}).map(([key, hard]) => (
                <p key={key}>
                  {key} · used {q.used?.[key] ?? "unknown"} / hard {hard}
                </p>
              ))}
            </section>
          ))}
          {quotas.next && <button onClick={() => void quota(quotas.next)}>Next quota page</button>}
        </>
      )}
      <h3>My access</h3>
      <button onClick={() => void check()}>Check my access</button>
      {ae && <p role="alert">{ae}</p>}
      {access && (
        <>
          <p>List Pods · {access.pods}</p>
          <p>List resource quotas · {access.quotas}</p>
          <p>Delete this Project · {access.delete}</p>
          <time>{access.observedAt}</time>
        </>
      )}
    </>
  );
}
function Action({
  mode,
  connection,
  project,
  close,
  submit,
}: {
  mode: Mode;
  connection: ConnectionRef;
  project?: Project;
  close: () => void;
  submit: Submit;
}) {
  const deleting = mode === "delete";
  const [name, setName] = useState(deleting ? (project?.resource.name ?? "") : "");
  const [display, setDisplay] = useState("");
  const [description, setDescription] = useState("");
  const [ack, setAck] = useState("");
  const [error, fail] = useState("");
  const [busy, setBusy] = useState(false);
  const [review, setReview] = useState<ProjectRequestPreview | DeletePreview>();
  const token = useRef("");
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (token.current) void (mode === "delete" ? api.RevokeDelete : api.RevokeProjectRequest)(token.current);
    };
  }, [mode]);
  async function prepare() {
    if (deleting && !project) return;
    setBusy(true);
    fail("");
    try {
      const p = deleting
        ? await api.PrepareDelete(crypto.randomUUID(), { resource: project!.resource, sourceVersion: project!.version })
        : await api.PrepareProjectRequest(crypto.randomUUID(), connection, { name, displayName: display, description });
      if (!alive.current) {
        void (deleting ? api.RevokeDelete : api.RevokeProjectRequest)(p.token);
        return;
      }
      token.current = p.token;
      setReview(p);
    } catch (e) {
      fail(message(e));
    } finally {
      setBusy(false);
    }
  }
  const typed = deleting || (review && "typed" in review && review.typed);
  return (
    <Modal title={deleting ? "Delete project" : "Request project"} onCancel={close}>
      <fieldset disabled={!!review || busy || deleting}>
        <label>
          Project name <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        {!deleting && (
          <>
            <label>
              Display name <input value={display} onChange={(e) => setDisplay(e.target.value)} />
            </label>
            <label>
              Description <textarea value={description} onChange={(e) => setDescription(e.target.value)} />
            </label>
          </>
        )}
      </fieldset>
      <p>
        {deleting
          ? "Deleting this Project removes its namespace and contents, including resources you cannot list."
          : "The cluster’s configured Project template may create resources and grant access."}
      </p>
      {error && <p role="alert">{error}</p>}
      {review ? (
        <>
          <p>
            {review.cluster} · cluster scope · {name}
          </p>
          {deleting && project && (
            <p>
              {project.phase} · UID {project.resource.uid} · version {project.version}
            </p>
          )}
          {!deleting && (
            <button
              onClick={() => {
                void api.RevokeProjectRequest(token.current);
                token.current = "";
                setReview(undefined);
                setAck("");
              }}
            >
              Edit request
            </button>
          )}
          {typed && (
            <label>
              Type {name} to confirm <input value={ack} onChange={(e) => setAck(e.target.value)} />
            </label>
          )}
          <button
            disabled={typed && ack !== name}
            onClick={() => {
              const t = token.current;
              if (!t) return;
              token.current = "";
              submit(mode, t, ack);
              close();
            }}
          >
            Confirm {deleting ? "deletion" : "request"}
          </button>
        </>
      ) : (
        <button disabled={busy || !name} onClick={() => void prepare()}>
          Review {deleting ? "deletion" : "request"}
        </button>
      )}
    </Modal>
  );
}
