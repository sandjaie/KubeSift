/*
Purpose: Browse one discovered OpenShift workflow in the selected workspace scope.
Used by: The workspace and command palette destinations.
Responsibilities: Separate named lookup, bounded inventory and related Service inspection.
Important: All reads carry the original connection and scope. Leaving discards outstanding replies.
*/
import { useState, useEffect, useRef } from "react";
import { api, message, type ConnectionRef } from "../connections/api";
import type { APIResourceDescriptor, WorkflowItem, WorkflowQuery, RouteService } from "../../bindings/internal/kube/models";
import { readRequest } from "../workloads/api";
import { ResourceSplit } from "../workspace/ResourceSplit";
import type { ComponentProps } from "react";
import { RouteAction, type RouteActionName, type SubmitWorkflow } from "./RouteAction";
import { BuildControls } from "./BuildControls";
import type { LogSessionsController } from "../sessions/useLogSessions";
import { OperatorControls } from "./OperatorControls";
import type { OperationResult } from "../operations/api";
export type Workflow = "Routes" | "Builds" | "Operators";
export function destination(d: APIResourceDescriptor): Workflow {
  if (d.group === "route.openshift.io") return "Routes";
  if (d.group === "build.openshift.io") return "Builds";
  return "Operators";
}
export function WorkflowWorkspace(props: {
  connection: ConnectionRef;
  namespace: string;
  projectUID: string;
  resources: APIResourceDescriptor[];
  submit: SubmitWorkflow;
  actions: Record<string, boolean | undefined>;
  openGeneric: (d: APIResourceDescriptor) => void;
  logs: LogSessionsController;
  results: OperationResult[];
  split: Omit<ComponentProps<typeof ResourceSplit>, "children">;
}) {
  const [kind, choose] = useState(props.resources[0]?.kind);
  const desc = props.resources.find((d) => d.kind === kind) ?? props.resources[0];
  return desc && <WorkflowBrowser key={desc.kind} {...props} desc={desc} choose={choose} />;
}
function WorkflowBrowser({
  connection,
  namespace,
  projectUID,
  resources,
  desc,
  choose,
  split,
  submit,
  actions,
  logs,
  results,
  openGeneric,
}: {
  connection: ConnectionRef;
  namespace: string;
  projectUID: string;
  resources: APIResourceDescriptor[];
  submit: SubmitWorkflow;
  actions: Record<string, boolean | undefined>;
  openGeneric: (d: APIResourceDescriptor) => void;
  logs: LogSessionsController;
  results: OperationResult[];
  desc: APIResourceDescriptor;
  choose: (k: string) => void;
  split: Omit<ComponentProps<typeof ResourceSplit>, "children">;
}) {
  const [items, setItems] = useState<WorkflowItem[]>([]),
    [next, setNext] = useState(""),
    [listError, listFail] = useState(""),
    [rev, refresh] = useState(0);
  const [query, setQuery] = useState(""),
    [item, select] = useState<WorkflowItem>(),
    [service, setService] = useState<RouteService>(),
    [error, fail] = useState("");
  const serial = useRef(0),
    cancel = useRef<() => void>(() => {}),
    pageCancel = useRef<() => void>(() => {}),
    pageSerial = useRef(0);
  const [action, setAction] = useState<RouteActionName>();
  const ns = desc.namespaced ? namespace : "",
    uid = desc.namespaced ? projectUID : "";
  const target: WorkflowQuery = { connection, namespace: ns, projectUID: uid, kind: desc.kind, name: "", uid: "", cursor: "" };
  async function load(cursor: string, prior: WorkflowItem[]) {
    const n = ++pageSerial.current;
    pageCancel.current();
    setNext("");
    listFail("");
    const request = readRequest((id) => api.ReadWorkflow(id, { ...target, cursor }));
    pageCancel.current = request.cancel;
    try {
      const page = await request.promise;
      if (n !== pageSerial.current) return;
      const combined = prior.concat(page.items ?? []);
      setItems(combined.slice(0, 500));
      setNext(combined.length < 500 ? page.next : "");
      if (combined.length > 500 || (page.next && combined.length >= 500)) listFail("Partial inventory. Display limit reached.");
    } catch (e) {
      if (n === pageSerial.current) listFail(message(e));
    }
  }
  useEffect(() => {
    setItems([]);
    setNext("");
    if (desc.list) void load("", []);
    return () => {
      pageSerial.current++;
      pageCancel.current();
      serial.current++;
      cancel.current();
    };
  }, [rev, desc.list, desc.kind, ns, uid, connection.id, connection.generation]);
  async function inspect(name: string, expected = "") {
    const n = ++serial.current;
    cancel.current();
    setService(undefined);
    setLinked(undefined);
    setAction(undefined);
    select(undefined);
    fail("");
    const req = readRequest((id) => api.ReadWorkflow(id, { ...target, name, uid: expected }));
    cancel.current = req.cancel;
    try {
      const p = await req.promise;
      if (n === serial.current) select(p.items?.[0]);
    } catch (e) {
      if (n === serial.current) fail(message(e));
    }
  }
  async function backend(name: string) {
    if (!item) return;
    const n = ++serial.current;
    cancel.current();
    fail("");
    const req = readRequest((id) => api.ReadRouteService(id, { ...target, name: item.resource.name, uid: item.resource.uid }, name));
    cancel.current = req.cancel;
    try {
      const result = await req.promise;
      if (n === serial.current) setService(result);
    } catch (e) {
      if (n === serial.current) fail(message(e));
    }
  }
  const [linked, setLinked] = useState<WorkflowItem>();
  async function openLink(link: NonNullable<WorkflowItem["links"]>[number]) {
    if (!item) return;
    const n = ++serial.current;
    cancel.current();
    fail("");
    const req = readRequest((id) => api.ReadWorkflowLink(id, { ...target, name: item.resource.name, uid: item.resource.uid }, link));
    cancel.current = req.cancel;
    try {
      const v = await req.promise;
      if (n === serial.current) setLinked(v);
    } catch (e) {
      if (n === serial.current) fail(message(e));
    }
  }
  const [instanceName, setInstanceName] = useState(""),
    [instanceNS, setInstanceNS] = useState(namespace);
  async function openInstance(a: { kind: string; name: string; version: string }) {
    if (!item) return;
    const n = ++serial.current;
    cancel.current();
    fail("");
    const req = readRequest((id) =>
      api.ReadOperatorInstance(id, { ...target, name: item.resource.name, uid: item.resource.uid }, a, instanceNS, instanceName),
    );
    cancel.current = req.cancel;
    try {
      const result = await req.promise;
      if (n === serial.current) setLinked(result);
    } catch (e) {
      if (n === serial.current) fail(message(e));
    }
  }
  const fields = item ? JSON.parse(item.fields) : null;
  return (
    <ResourceSplit {...split}>
      <section className="list-pane">
        <div className="pane-heading">
          <h2>{destination(desc)}</h2>
          {desc.kind === "Route" && desc.create && <button onClick={() => setAction("create-route")}>Create Route</button>}
          <button
            onClick={() => {
              select(undefined);
              setService(undefined);
              setLinked(undefined);
              setAction(undefined);
              serial.current++;
              cancel.current();
              refresh((v) => v + 1);
            }}
          >
            Refresh
          </button>
        </div>
        {resources.length > 1 && (
          <label>
            Resource{" "}
            <select value={desc.kind} onChange={(e) => choose(e.target.value)}>
              {resources.map((r) => (
                <option key={r.kind}>{r.kind}</option>
              ))}
            </select>
          </label>
        )}
        <p>{ns || "Cluster scope"}</p>
        {destination(desc) === "Operators" && (
          <p>
            {resources.some((r) => r.group === "operators.coreos.com") ? "OLM Classic served" : "Classic APIs unavailable"} ·{" "}
            {resources.some((r) => r.group === "olm.operatorframework.io") ? "OLM v1 served" : "v1 APIs unavailable"}
          </p>
        )}
        {desc.group === "olm.operatorframework.io" && desc.mode === "generic" && (
          <button onClick={() => openGeneric(desc)}>Inspect in API resources</button>
        )}

        {desc.get && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void inspect(query);
            }}
          >
            <label>
              {desc.kind} name{" "}
              <input autoCapitalize="none" autoCorrect="off" spellCheck={false} value={query} onChange={(e) => setQuery(e.target.value)} />
            </label>
            <button disabled={!query}>Look up</button>
          </form>
        )}
        {listError && <p role="alert">{listError}</p>}
        <table>
          <tbody>
            {items.map((row) => (
              <tr key={row.resource.uid}>
                <td>
                  <button
                    onClick={() => {
                      if (desc.get) void inspect(row.resource.name, row.resource.uid);
                      else {
                        serial.current++;
                        cancel.current();
                        setService(undefined);
                        setLinked(undefined);
                        setAction(undefined);
                        fail("");
                        select(row);
                      }
                    }}
                  >
                    {row.resource.name}
                  </button>
                </td>
                {desc.kind === "Route" && (
                  <td>
                    <RouteSummary fields={row.fields} />
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
        {next && <button onClick={() => void load(next, items)}>Next page</button>}
      </section>
      <section className="detail-pane">
        {error && <p role="alert">{error}</p>}
        {destination(desc) === "Operators" && (
          <OperatorControls
            key={item?.resource.uid ?? "list"}
            item={item}
            target={target}
            resources={resources}
            desc={desc}
            submit={submit}
            results={results}
          />
        )}
        {linked ? (
          <>
            <button onClick={() => setLinked(undefined)}>Back to {desc.kind}</button>
            <h2>{linked.resource.name}</h2>
            <p>
              {linked.resource.namespace} · {linked.resource.kind} · {linked.resource.uid}
            </p>
            <pre>{linked.fields}</pre>
          </>
        ) : service ? (
          <>
            <button onClick={() => setService(undefined)}>Back to Route</button>
            <h2>{service.resource.name}</h2>
            <p>Service · {service.type}</p>
            <p>{service.ports?.join(", ")}</p>
          </>
        ) : (
          item && (
            <>
              <h2>{item.resource.name}</h2>
              <p>
                {desc.kind} · {item.resource.uid} · version {item.version}
              </p>
              {desc.kind === "Route" && (
                <>
                  <h3>Backend Services</h3>
                  {[fields.spec.to, ...(fields.spec.alternateBackends ?? [])].map((b, i) => (
                    <button key={i} disabled={!desc.get || (b.kind && b.kind !== "Service")} onClick={() => void backend(b.name)}>
                      {b.name}
                      {b.weight !== undefined ? ` · weight ${b.weight}` : ""}
                    </button>
                  ))}
                  <h3>Router admission</h3>
                  {!fields.status.ingress?.length && <p>No router observation.</p>}
                </>
              )}
              {item.links?.map((l) => (
                <button key={JSON.stringify(l)} onClick={() => void openLink(l)}>
                  Open current {l.kind} · {l.namespace}/{l.name}
                </button>
              ))}
              {desc.kind === "ClusterServiceVersion" && !!fields.Owned?.length && !fields.copied && (
                <fieldset>
                  <legend>Inspect a managed instance</legend>
                  <label>
                    Instance namespace (empty for cluster scope)
                    <input
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      value={instanceNS}
                      onChange={(e) => setInstanceNS(e.target.value)}
                    />
                  </label>
                  <label>
                    Instance name
                    <input
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      value={instanceName}
                      onChange={(e) => setInstanceName(e.target.value)}
                    />
                  </label>
                  {(fields.Owned as { name: string; kind: string; version: string }[]).map((a) => (
                    <button key={a.name + a.version} disabled={!instanceName} onClick={() => void openInstance(a)}>
                      Check {a.kind} ownership
                    </button>
                  ))}
                </fieldset>
              )}
              <WorkflowFacts fields={fields} kind={desc.kind} />
              {destination(desc) === "Builds" && desc.get && (
                <BuildControls
                  key={item.resource.uid}
                  item={item}
                  target={target}
                  actions={actions}
                  desc={desc}
                  logs={logs}
                  submit={submit}
                />
              )}
              {desc.kind === "Route" && desc.get && (
                <>
                  {desc.patch && <button onClick={() => setAction("edit-route")}>Edit Route</button>}
                  {desc.delete && <button onClick={() => setAction("delete-route")}>Delete Route</button>}
                </>
              )}
            </>
          )
        )}
        {action && <RouteAction action={action} target={target} item={item} submit={submit} close={() => setAction(undefined)} />}
      </section>
    </ResourceSplit>
  );
}

function WorkflowFacts({ fields, kind }: { fields: Record<string, unknown>; kind: string }) {
  const facts: [string, unknown][] = kind === "Route" ? Object.entries((fields.spec ?? {}) as object) : Object.entries(fields);
  return (
    <div className="evidence-section">
      <dl className="pod-facts">
        {facts
          .filter(([, v]) => v !== null && v !== undefined && v !== "")
          .map(([key, v]) => (
            <div key={key}>
              <dt>{key.replace(/([a-z])([A-Z])/g, "$1 $2")}</dt>
              <dd>{typeof v === "object" ? <pre>{JSON.stringify(v, null, 2)}</pre> : String(v)}</dd>
            </div>
          ))}
      </dl>
      {kind === "Route" && <pre>{JSON.stringify(fields.status, null, 2)}</pre>}
    </div>
  );
}

function RouteSummary({ fields }: { fields: string }) {
  const { spec, status } = JSON.parse(fields);
  const routers = (status.ingress ?? []) as { routerName: string; conditions?: { type: string; status: string }[] }[];
  return (
    <>
      {spec.host || "Assigned host"}
      {spec.path || "/"} · {spec.to.name} ·{" "}
      {routers.length
        ? routers
            .map(
              (r) =>
                r.routerName +
                " " +
                (r.conditions
                  ?.filter((c) => c.type === "Admitted")
                  .map((c) => c.status)
                  .join(", ") || "unknown"),
            )
            .join(" / ")
        : "No router observation"}
    </>
  );
}
