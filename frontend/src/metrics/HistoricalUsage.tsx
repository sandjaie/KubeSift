/*
Purpose: Show on-demand CPU and working-set memory history for one selected resource.
Used by: Pod and Node Overview below Current usage.
Responsibilities: Confirm provider scope, cancel obsolete reads and display evaluation gaps.
Important: Credentials are submitted only by the source form and are never returned.
*/
import { useEffect, useLayoutEffect, useState, useRef } from "react";
import type { FormEvent } from "react";
import type { HistorySource, HistoricalMetrics, HistoryMetric, ResourceRef } from "../../bindings/internal/kube/models";
import { onVisibility } from "./api";
import { message } from "../connections/api";
import { workloadAPI as api, readRequest } from "../workloads/api";
type Target = { resource: ResourceRef; connection: string; containers: string[] };
const utc = (seconds: number) => new Date(seconds * 1000).toISOString().replace("T", " ").replace(".000Z", " UTC");
const states: Record<string, string> = {
  missing: "No matching samples",
  ambiguous: "Multiple matching series",
  unsupported: "Unsupported provider response",
  capacity: "Response exceeded the limit",
  timeout: "Provider timed out",
  transport: "Provider request failed",
  busy: "Provider is busy",
  tls: "TLS verification failed. Check the hostname and selected CA.",
};
function Chart({ name, metric, result, cpu }: { name: string; metric: HistoryMetric; result: HistoricalMetrics; cpu: boolean }) {
  const points = metric.points ?? [],
    values = new Map(points.map((p) => [p.time, p.value]));
  const display = (v: number) =>
    cpu
      ? `${v.toLocaleString(undefined, { maximumSignificantDigits: 5 })} mCPU`
      : `${(v / 1048576).toLocaleString(undefined, { maximumSignificantDigits: 5 })} MiB`;
  const max = Math.max(...points.map((p) => p.value), 1);
  const x = (t: number) => 4 + (312 * (t - result.start)) / (result.end - result.start),
    y = (v: number) => 68 - (64 * v) / max;
  const last = points[points.length - 1];
  return (
    <section>
      <h4>{name}</h4>
      {metric.qualified && <p>Provider qualified this result. Coverage may be partial.</p>}
      {metric.state !== "available" && <p>{states[metric.state] ?? "Unavailable"}</p>}
      {last && (
        <>
          <p>
            {utc(points[0].time)} – {utc(last.time)} · Last evaluation {display(last.value)}
            {result.end - last.time > 2 * result.step ? " · Stale trailing coverage" : ""}
          </p>
          <svg
            viewBox="0 0 320 72"
            role="img"
            aria-label={name + " history"}
            style={{ width: "100%", maxHeight: 100, color: "var(--accent, currentColor)" }}
          >
            <path
              fill="none"
              stroke="currentColor"
              d={points
                .map((p, i) => (i && points[i - 1].time === p.time - result.step ? "L" : "M") + x(p.time) + " " + y(p.value))
                .join(" ")}
            />
            {points.map((p) => (
              <circle key={p.time} cx={x(p.time)} cy={y(p.value)} r="1.5" fill="currentColor" />
            ))}
          </svg>
        </>
      )}
      <details>
        <summary>{name} values and gaps</summary>
        <table>
          <thead>
            <tr>
              <th>Evaluation (UTC)</th>
              <th>{cpu ? "CPU (mCPU)" : "Working set (MiB)"}</th>
            </tr>
          </thead>
          <tbody>
            {Array.from({ length: 1 + (result.end - result.start) / result.step }, (_, i) => {
              const t = result.start + i * result.step,
                v = values.get(t);
              return (
                <tr key={t}>
                  <td>{utc(t)}</td>
                  <td>{v === undefined ? "Gap" : display(v)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </details>
    </section>
  );
}
function SourceForm({ resource, source, changed }: { resource: ResourceRef; source: HistorySource; changed: (s: HistorySource) => void }) {
  const [origin, setOrigin] = useState(""),
    [ca, setCA] = useState(""),
    [scope, setScope] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const suggestion = useRef<ReturnType<typeof readRequest<string>> | null>(null);
  useEffect(() => () => suggestion.current?.cancel(), []);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget,
      data = new FormData(form);
    const bearer = String(data.get("token") ?? "");
    (form.elements.namedItem("token") as HTMLInputElement).value = "";
    setBusy(true);
    setError("");
    try {
      changed(
        await api.SetHistorySource(resource.connection, source.revision, {
          origin,
          caFile: ca,
          mode: String(data.get("mode")),
          singleCluster: scope === "single",
          cluster: scope === "shared" ? String(data.get("cluster")) : "",
          bearer,
        }),
      );
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={save}>
      <fieldset disabled={busy}>
        <legend>{source.origin ? "Replace source" : "Configure history source"}</legend>
        <label>
          HTTPS origin <input type="url" required value={origin} onChange={(e) => setOrigin(e.target.value)} />
        </label>
        <button
          type="button"
          onClick={async () => {
            setBusy(true);
            setError("");
            const read = readRequest((id) => api.SuggestHistorySource(id, resource.connection));
            suggestion.current = read;
            try {
              setOrigin(await read.promise);
            } catch (e) {
              setError(message(e));
            } finally {
              setBusy(false);
            }
          }}
        >
          Suggest OpenShift Route
        </button>
        <label>
          Endpoint access{" "}
          <select name="mode">
            <option value="cluster">Cluster-wide</option>
            <option value="project">OpenShift project tenancy</option>
          </select>
        </label>
        <label>
          Cluster scope{" "}
          <select required value={scope} onChange={(e) => setScope(e.target.value)}>
            <option value="">Choose scope</option>
            <option value="single">I confirm this endpoint serves only this cluster</option>
            <option value="shared">Shared endpoint with cluster label</option>
          </select>
        </label>
        {scope === "shared" && (
          <label>
            Exact cluster label <input name="cluster" autoCapitalize="none" autoCorrect="off" spellCheck={false} required />
          </label>
        )}
        <label>
          Optional provider bearer <input key={origin} name="token" type="password" autoComplete="off" />
        </label>
        <button
          type="button"
          onClick={async () => {
            try {
              setCA(await api.PickHistoryCA());
            } catch (e) {
              setError(message(e));
            }
          }}
        >
          Select private CA…
        </button>
        {ca && <span>{ca.split("/").pop()}</span>}
        <button type="submit">Confirm source</button>
      </fieldset>
      {error && <p role="alert">{error}</p>}
    </form>
  );
}
function HistoryRead({
  resource,
  source,
  container,
  preset,
}: {
  resource: ResourceRef;
  source: HistorySource;
  container: string;
  preset: string;
}) {
  const [result, setResult] = useState<HistoricalMetrics>(),
    [error, setError] = useState("");
  useLayoutEffect(() => {
    let alive = true;
    let reservation: string | undefined;
    const cancel = (id: string) => void api.CancelWorkloadRead(id).catch(() => {});
    async function load() {
      try {
        const id = await api.PrepareHistoricalMetrics();
        if (!alive) {
          cancel(id);
          return;
        }
        reservation = id;
        const data = await api.ReadHistoricalMetrics(id, { resource, sourceID: source.revision, container, preset });
        if (alive) setResult(data);
      } catch (e) {
        if (alive) setError(message(e));
      }
    }
    void load();
    return () => {
      alive = false;
      if (reservation) cancel(reservation);
    };
  }, [resource, source.revision, container, preset]);
  if (error) return <p role="alert">{error}</p>;
  if (!result) return <p role="status">Loading history…</p>;
  return (
    <>
      <p>
        {utc(result.start)} – {utc(result.end)} · Step {result.step}s · Fetched {utc(result.fetchedAt)}
      </p>
      <p>
        Matched by source labels and current object lifetime. The first five minutes after creation are excluded. CPU uses a five-minute
        rate. Memory can repeat the last sample within five minutes. Scrape freshness is unknown.
      </p>
      <Chart name="CPU" result={result} metric={result.cpu} cpu />
      <Chart name="Working-set memory" result={result} metric={result.memory} cpu={false} />
    </>
  );
}
function Panel({ resource: target, containers, connection, visible }: Target & { visible: boolean }) {
  const [resource] = useState(target);
  const [source, setSource] = useState<HistorySource>(),
    [error, setError] = useState(""),
    [editing, setEditing] = useState(false),
    [range, setRange] = useState("1h"),
    [selected, setSelected] = useState(""),
    [refresh, setRefresh] = useState(0);
  const container = containers.includes(selected) ? selected : (containers[0] ?? "");
  useEffect(() => {
    let alive = true;
    api.HistorySource(resource.connection).then(
      (s) => {
        if (alive) setSource(s);
      },
      (e) => {
        if (alive) setError(message(e));
      },
    );
    return () => {
      alive = false;
    };
  }, [resource.connection]);
  return (
    <>
      <p>
        {connection} / {[resource.namespace, resource.kind, resource.name].filter(Boolean).join(" / ")}
      </p>
      {error && <p role="alert">{error}</p>}
      {source &&
        (source.origin ? (
          <>
            <p>
              {source.origin} · {source.mode === "project" ? "Project tenancy" : "Cluster-wide"} ·{" "}
              {source.cluster ? "cluster = " + source.cluster : "Confirmed single cluster"} ·{" "}
              {source.authenticated ? "Bearer configured" : "Anonymous"}
            </p>
            <button onClick={() => setEditing(!editing)}>{editing ? "Cancel replacement" : "Replace source"}</button>{" "}
            <button
              onClick={async () => {
                try {
                  setSource(await api.ClearHistorySource(resource.connection, source.revision));
                  setEditing(false);
                } catch (e) {
                  setError(message(e));
                }
              }}
            >
              Remove source
            </button>
          </>
        ) : (
          <p>No history source configured.</p>
        ))}
      {source && (!source.origin || editing) && (
        <SourceForm
          resource={resource}
          source={source}
          changed={(s) => {
            setSource(s);
            setEditing(false);
            setError("");
          }}
        />
      )}
      {source?.origin &&
        !editing &&
        (source.mode === "project" && resource.kind === "Node" ? (
          <p>Project endpoints only support Pod history.</p>
        ) : (
          <>
            {resource.kind === "Pod" && (
              <label>
                Container{" "}
                <select value={container} onChange={(e) => setSelected(e.target.value)}>
                  {containers.map((c) => (
                    <option key={c}>{c}</option>
                  ))}
                </select>
              </label>
            )}
            <label>
              Range{" "}
              <select aria-label="History range" value={range} onChange={(e) => setRange(e.target.value)}>
                {["1h", "6h", "24h"].map((r) => (
                  <option key={r}>{r}</option>
                ))}
              </select>
            </label>
            <button onClick={() => setRefresh((v) => v + 1)}>Refresh history</button>
            {!visible && <p role="status">History paused while the window is inactive.</p>}
            {visible && (container || resource.kind === "Node") && (
              <HistoryRead
                key={JSON.stringify([source.revision, container, range, refresh])}
                resource={resource}
                source={source}
                container={container}
                preset={range}
              />
            )}
          </>
        ))}
    </>
  );
}
export function HistoricalUsage(props: Target) {
  const [open, setOpen] = useState(false),
    [native, setNative] = useState(true),
    [visible, setVisible] = useState(document.visibilityState !== "hidden");
  useEffect(() => {
    const change = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", change);
    const off = onVisibility(setNative);
    return () => {
      off();
      document.removeEventListener("visibilitychange", change);
    };
  }, []);
  return (
    <section className="historical-usage">
      <button aria-expanded={open} onClick={() => setOpen(!open)}>
        Historical usage
      </button>
      {open && <Panel key={JSON.stringify(props.resource)} {...props} visible={native && visible} />}
    </section>
  );
}
